import time

import httpx
import pytest
from thermocktat_client import FanSpeed, Mode, ThermocktatSync

BASE_URL = "http://localhost:8080"


@pytest.fixture
def http_client():
    """Raw httpx client, still used for endpoints the SDK doesn't expose (e.g. healthz)."""
    return httpx.Client(base_url=BASE_URL)


@pytest.fixture
def tmk(http_tmk_application):
    with ThermocktatSync.connect(BASE_URL) as client:
        yield client


def test_healthz(http_tmk_application, http_client):
    response = http_client.get("/healthz")
    assert response.status_code == 200


def test_get_snapshot(tmk):
    assert tmk.snapshot.device_id
    assert isinstance(tmk.snapshot.temperature_setpoint, (int, float))
    assert isinstance(tmk.snapshot.enabled, bool)
    assert tmk.snapshot.mode in set(Mode)


@pytest.mark.parametrize("value", [True, False])
def test_set_enabled(tmk, value):
    tmk.set_enabled(value)
    assert tmk.snapshot.enabled is value


def test_write_temperature_setpoint(tmk):
    setpoint = 20.0
    tmk.set_temperature_setpoint(setpoint)
    assert tmk.snapshot.temperature_setpoint == setpoint


@pytest.mark.parametrize("mode", list(Mode))
def test_write_mode(tmk, mode):
    tmk.set_mode(mode)
    assert tmk.snapshot.mode == mode


@pytest.mark.parametrize("fan_speed", list(FanSpeed))
def test_write_fan_speed(tmk, fan_speed):
    tmk.set_fan_speed(fan_speed)
    assert tmk.snapshot.fan_speed == fan_speed


def test_custom_port_configuration(tmk_run):
    custom_port = 8081
    custom_url = f"http://localhost:{custom_port}"
    with tmk_run(
        controller="http",
        extra_env={"TMK_CONTROLLERS_HTTP_ADDR": f":{custom_port}"},
    ):
        time.sleep(1)
        with ThermocktatSync.connect(custom_url) as client:
            new_setpoint = 22.5
            client.set_temperature_setpoint(new_setpoint)
            assert client.snapshot.temperature_setpoint == new_setpoint
