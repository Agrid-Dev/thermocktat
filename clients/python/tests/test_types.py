from thermocktat_client import FanSpeed, Mode, Snapshot


def make_snapshot(**overrides) -> Snapshot:
    defaults = {
        "device_id": "my-thermocktat",
        "enabled": True,
        "ambient_temperature": 20.5,
        "temperature_setpoint": 22.0,
        "temperature_setpoint_min": 16.0,
        "temperature_setpoint_max": 28.0,
        "mode": Mode.AUTO,
        "fan_speed": FanSpeed.AUTO,
    }
    return Snapshot(**(defaults | overrides))


def test_str_contains_device_id_and_key_state():
    snap = make_snapshot()
    rendered = str(snap)
    assert "my-thermocktat" in rendered
    assert "auto" in rendered
    assert "22" in rendered
    assert "20.5" in rendered


def test_str_shows_enabled_state():
    assert "on" in str(make_snapshot(enabled=True)).lower()
    assert "off" in str(make_snapshot(enabled=False)).lower()


def test_str_is_multiline_and_readable():
    rendered = str(make_snapshot())
    assert "\n" in rendered
    assert rendered == rendered.strip()


def test_repr_is_dataclass_default():
    snap = make_snapshot()
    assert repr(snap).startswith("Snapshot(")
