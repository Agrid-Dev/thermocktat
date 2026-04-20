import pytest


@pytest.fixture
def snapshot_response() -> dict:
    return {
        "device_id": "my-thermocktat",
        "enabled": True,
        "temperature_setpoint": 22,
        "temperature_setpoint_min": 16,
        "temperature_setpoint_max": 28,
        "mode": "auto",
        "fan_speed": "auto",
        "ambient_temperature": 20.98131495252262,
        "fault_code": 0,
    }
