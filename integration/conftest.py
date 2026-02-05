import os
import subprocess
import time
from contextlib import contextmanager
from typing import Literal

import pytest

ControllerType = Literal["http", "mqtt", "modbus"]


@contextmanager
def tmk_application(controller: ControllerType, addr: str):
    env = os.environ.copy()
    env["TMK_CONTROLLER"] = controller
    env["TMK_ADDR"] = addr

    # Regulator settings
    env["TMK_REGULATOR_MODE_CHANGE_HYSTERESIS"] = "2.0"
    env["TMK_REGULATOR_TARGET_HYSTERESIS"] = "1.0"
    print("starting process")
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


@pytest.fixture(scope="module")
def http_tmk_application():
    with tmk_application(controller="http", addr=":8080"):
        yield


@pytest.fixture(scope="module")
def modbus_tmk_application():
    with tmk_application(controller="modbus", addr="0.0.0.0:1502"):
        yield


@pytest.fixture(scope="module")
def mqtt_tmk_application():
    with tmk_application(controller="mqtt", addr="localhost:1883"):
        yield
