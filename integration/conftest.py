import os
import platform
import shutil
import socket
import subprocess
import tempfile
import time
from contextlib import contextmanager
from pathlib import Path
from typing import Literal

import docker
import docker.errors
import pytest

ControllerType = Literal["http", "mqtt", "modbus", "bacnet", "knx"]


def pytest_addoption(parser):
    parser.addoption(
        "--docker-image",
        default=None,
        help="Run tests against a Docker image instead of the local binary.",
    )


@pytest.fixture(scope="session")
def docker_image(request):
    """Return the Docker image name if --docker-image was passed, else None."""
    return request.config.getoption("--docker-image")


# --- Application launchers ---


@contextmanager
def tmk_application(
    controller: ControllerType,
    addr: str | None = None,
    extra_env: dict | None = None,
):
    env = os.environ.copy()
    env["TMK_CONTROLLER"] = controller
    if addr is not None:
        env["TMK_ADDR"] = addr

    # Regulator settings
    env["TMK_REGULATOR_MODE_CHANGE_HYSTERESIS"] = "2.0"
    env["TMK_REGULATOR_TARGET_HYSTERESIS"] = "1.0"

    if extra_env:
        env.update(extra_env)

    process = subprocess.Popen(
        ["./.bin/thermocktat"], env=env, stdout=None, stderr=None
    )
    try:
        # give the process a moment to start
        time.sleep(1)
        yield  # Tests will run while inside the context
    finally:
        print("terminating process")
        process.terminate()
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()


def _get_docker_client():
    """Get Docker client, skip tests if Docker is unavailable."""
    try:
        return docker.from_env()
    except docker.errors.DockerException:
        pytest.skip("Docker daemon not available")


@contextmanager
def tmk_docker_application(
    image: str,
    controller: ControllerType,
    addr: str | None = None,
    extra_env: dict | None = None,
):
    """Run thermocktat in a Docker container with --network host."""
    client = _get_docker_client()
    env: dict[str, str] = {
        "TMK_CONTROLLER": controller,
        "TMK_REGULATOR_MODE_CHANGE_HYSTERESIS": "2.0",
        "TMK_REGULATOR_TARGET_HYSTERESIS": "1.0",
    }
    if addr is not None:
        env["TMK_ADDR"] = addr
    if extra_env:
        env.update(extra_env)

    container = client.containers.run(
        image,
        detach=True,
        network_mode="host",
        environment=env,
        remove=True,
    )
    try:
        time.sleep(2)
        container.reload()
        if container.status != "running":
            logs = container.logs().decode()
            pytest.fail(f"Container failed to start: {container.status}\n{logs}")
        yield container
    finally:
        try:
            container.stop(timeout=5)
        except docker.errors.APIError:
            pass


@pytest.fixture(scope="session")
def tmk_run(docker_image):
    """Factory that returns the right context manager for the current mode."""
    if docker_image:

        def _run(controller, addr=None, extra_env=None):
            return tmk_docker_application(docker_image, controller, addr, extra_env)
    else:

        def _run(controller, addr=None, extra_env=None):
            return tmk_application(controller, addr, extra_env)

    return _run


# --- Protocol fixtures ---


@pytest.fixture(scope="module")
def http_tmk_application(tmk_run):
    with tmk_run(controller="http", addr=":8080"):
        yield


@pytest.fixture(scope="module")
def modbus_tmk_application(tmk_run):
    with tmk_run(controller="modbus", addr="0.0.0.0:1502"):
        yield


@pytest.fixture(scope="module")
def modbus_tmk_application_32bit(tmk_run):
    with tmk_run(
        controller="modbus",
        addr="0.0.0.0:1503",
        extra_env={"TMK_CONTROLLERS_MODBUS_REGISTER_COUNT": "2"},
    ):
        yield


@pytest.fixture(scope="module")
def knx_tmk_application(tmk_run):
    with tmk_run(
        controller="knx",
        addr="0.0.0.0:3671",
        extra_env={"TMK_CONTROLLERS_KNX_PUBLISH_INTERVAL": "1s"},
    ):
        yield


@pytest.fixture(scope="module")
def mqtt_tmk_application(tmk_run):
    with tmk_run(controller="mqtt", addr="localhost:1883"):
        yield


# --- Docker-based BACnet fixtures ---

_DOCKERFILE_DUT = Path(__file__).parent / "Dockerfile.dut"

BACNET_HOST_PORT = 47808


@pytest.fixture(scope="session")
def _linux_binary() -> Path:
    """Return a Linux/amd64 binary, cross-compiling on non-Linux hosts."""
    if platform.system() == "Linux":
        binary = Path(".bin/thermocktat")
        if not binary.exists():
            pytest.fail(
                "Binary not found at .bin/thermocktat. "
                "Run: CGO_ENABLED=0 go build -o .bin/thermocktat ../cmd/thermocktat"
            )
        return binary

    # Non-Linux: cross-compile for linux/amd64
    binary = Path(".bin/thermocktat-linux-amd64")
    binary.parent.mkdir(parents=True, exist_ok=True)
    try:
        subprocess.run(
            ["go", "build", "-o", str(binary), "../cmd/thermocktat"],
            env={**os.environ, "GOOS": "linux", "GOARCH": "amd64", "CGO_ENABLED": "0"},
            check=True,
            capture_output=True,
        )
    except FileNotFoundError:
        pytest.skip("go not found in PATH; cannot cross-compile for Linux")
    except subprocess.CalledProcessError as e:
        pytest.fail(f"go build failed:\n{e.stderr.decode()}")
    return binary


@pytest.fixture(scope="module")
def bacnet_tmk_container(docker_image, request):
    """Run the BACnet DUT — from the provided Docker image or a scratch build."""
    if docker_image:
        with tmk_docker_application(
            docker_image, "bacnet", "0.0.0.0:47808"
        ) as container:
            yield container
        return

    # Binary mode: build a FROM-scratch image from the cross-compiled binary
    linux_binary = request.getfixturevalue("_linux_binary")
    client = _get_docker_client()

    image_tag = "thermocktat:integration-test"

    with tempfile.TemporaryDirectory() as ctx:
        shutil.copy(linux_binary, Path(ctx) / "thermocktat")
        shutil.copy(_DOCKERFILE_DUT, Path(ctx) / "Dockerfile")
        try:
            client.images.build(path=ctx, tag=image_tag, rm=True)
        except docker.errors.BuildError as e:
            pytest.fail(f"Docker image build failed: {e}")
        except docker.errors.DockerException as e:
            pytest.skip(f"Docker error building image: {e}")

    container = client.containers.run(
        image_tag,
        detach=True,
        ports={"47808/udp": BACNET_HOST_PORT},
        environment={
            "TMK_CONTROLLER": "bacnet",
            "TMK_ADDR": "0.0.0.0:47808",
        },
        remove=True,
    )

    try:
        time.sleep(2)

        container.reload()
        if container.status != "running":
            pytest.fail(f"Container failed to start: {container.status}")

        yield container
    finally:
        try:
            container.stop(timeout=5)
        except docker.errors.APIError:
            pass


@pytest.fixture
def bacnet_socket_docker(bacnet_tmk_container):
    """Provide a UDP socket connected to the BACnet controller via port mapping."""
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.settimeout(2.0)
    sock.connect(("127.0.0.1", BACNET_HOST_PORT))
    try:
        yield sock
    finally:
        sock.close()
