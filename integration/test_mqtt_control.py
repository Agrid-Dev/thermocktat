import json
import time

import paho.mqtt.client as mqtt
import pytest

modes = ["heat", "cool", "fan", "auto"]
BASE_TOPIC = "thermocktat/default"


@pytest.fixture
def mqtt_client():
    """MQTT client connected to the local broker, with network loop running."""
    client = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2)
    # Match the broker URL used in the app config: tcp://localhost:1883
    client.connect("localhost", 1883, keepalive=60)
    client.loop_start()
    try:
        yield client
    finally:
        client.loop_stop()
        client.disconnect()


@pytest.mark.parametrize("tmk_application", ["mqtt"], indirect=True)
def test_connects_to_broker(tmk_application, mqtt_client):
    assert True


def _get_snapshot(mqtt_client, base_topic: str) -> dict | None:
    payload = None

    def on_message(client, userdata, msg):  # noqa: ARG001
        nonlocal payload
        payload = json.loads(msg.payload)

    mqtt_client.on_message = on_message
    mqtt_client.subscribe(f"{base_topic}/snapshot")
    mqtt_client.publish(f"{base_topic}/get/snapshot", b"{}")

    for _ in range(30):
        if payload is not None:
            break
        time.sleep(0.1)

    return payload


@pytest.mark.parametrize("tmk_application", ["mqtt"], indirect=True)
def test_get_snapshot(tmk_application, mqtt_client):
    snapshot = _get_snapshot(mqtt_client, BASE_TOPIC)
    assert snapshot is not None
    assert snapshot["device_id"] == "default"
    assert isinstance(snapshot["temperature_setpoint"], (int, float))
    assert isinstance(snapshot["enabled"], bool)
    assert snapshot["mode"] in modes


@pytest.mark.parametrize("tmk_application", ["mqtt"], indirect=True)
@pytest.mark.parametrize("value", [True, False])
def test_set_enabled(tmk_application, mqtt_client, value):
    mqtt_client.publish(
        f"{BASE_TOPIC}/set/enabled", json.dumps({"value": value}).encode()
    )
    snapshot = _get_snapshot(mqtt_client, BASE_TOPIC)
    assert snapshot is not None
    assert snapshot["enabled"] is value


@pytest.mark.parametrize("tmk_application", ["mqtt"], indirect=True)
def test_write_temperature_setpoint(tmk_application, mqtt_client):
    setpoint = 20.0
    mqtt_client.publish(
        f"{BASE_TOPIC}/set/temperature_setpoint",
        json.dumps({"value": setpoint}).encode(),
    )
    snapshot = _get_snapshot(mqtt_client, BASE_TOPIC)
    assert snapshot is not None
    assert snapshot["temperature_setpoint"] == setpoint


@pytest.mark.parametrize("tmk_application", ["mqtt"], indirect=True)
@pytest.mark.parametrize("mode", modes)
def test_write_mode(tmk_application, mqtt_client, mode):
    mqtt_client.publish(
        f"{BASE_TOPIC}/set/mode",
        json.dumps({"value": mode}).encode(),
    )
    snapshot = _get_snapshot(mqtt_client, BASE_TOPIC)
    assert snapshot is not None
    assert snapshot["mode"] == mode
