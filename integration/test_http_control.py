import os
import subprocess
import time

import httpx
import pytest

modes: set[str] = {"fan", "heat", "cool", "auto"}


@pytest.fixture
def http_client():
    return httpx.Client(base_url="http://localhost:8080")


@pytest.mark.parametrize("tmk_application", ["http"], indirect=True)
def test_healthz(tmk_application, http_client):
    response = http_client.get("/healthz")
    assert response.status_code == 200


def test_get_snapshot(tmk_application, http_client):
    response = http_client.get("/v1")
    assert response.status_code == 200
    snapshot = response.json()
    assert "device_id" in snapshot
    assert isinstance(snapshot["temperature_setpoint"], (int, float))
    assert isinstance(snapshot["enabled"], bool)
    assert snapshot["mode"] in modes


@pytest.mark.parametrize("tmk_application", ["http"], indirect=True)
@pytest.mark.parametrize("value", [True, False])
def test_set_enabled(tmk_application, http_client, value):
    response = http_client.post("/v1/enabled", json={"value": value})
    assert response.status_code == 200
    snapshot = response.json()
    assert snapshot["enabled"] is value


@pytest.mark.parametrize("tmk_application", ["http"], indirect=True)
def test_write_temperature_setpoint(tmk_application, http_client):
    setpoint = 20.0
    write_response = http_client.post(
        "/v1/temperature_setpoint", json={"value": setpoint}
    )
    assert write_response.status_code == 200
    snapshot = write_response.json()
    assert snapshot["temperature_setpoint"] == setpoint


@pytest.mark.parametrize("tmk_application", ["http"], indirect=True)
@pytest.mark.parametrize("mode", modes)
def test_write_mode(tmk_application, http_client, mode):
    write_response = http_client.post("/v1/mode", json={"value": mode})
    assert write_response.status_code == 200
    snapshot = write_response.json()
    assert snapshot["mode"] == mode


def test_custom_port_configuration():
    custom_port = 8081
    custom_url = f"http://localhost:{custom_port}"
    env = os.environ.copy()
    env["TMK_CONTROLLER"] = "http"
    env["TMK_CONTROLLERS_HTTP_ADDR"] = f":{custom_port}"
    env["TMK_REGULATOR_MODE_CHANGE_HYSTERESIS"] = "2.0"
    env["TMK_REGULATOR_TARGET_HYSTERESIS"] = "1.0"

    process = subprocess.Popen(
        ["./.bin/thermocktat"], env=env, stdout=None, stderr=None
    )
    try:
        time.sleep(1)
        with httpx.Client(base_url=custom_url) as client:
            new_setpoint = 22.5
            response = client.post(
                "/v1/temperature_setpoint", json={"value": new_setpoint}
            )
            assert response.status_code == 200
            snapshot = response.json()
            assert snapshot["temperature_setpoint"] == new_setpoint

    finally:
        process.terminate()
        process.wait()
