# MQTT controller

## Configuration

Example:
```yaml
controllers:
  http:
    enabled: true
    addr: ":8080"
  mqtt:
    enabled: true
    broker_url: "tcp://localhost:1883"
    qos: 0
    retain_snapshot: true
    publish_interval: 1s # will publish snapshot every interval (only if changed)
    base_topic: "room101" # optional : to override default base topic = thermocktat/{device_id}
    username: rubeus # if the broker requires authentication
    password: secret-password
```

## API

### Published topics

The controller publishes the full thermostat snapshot to:

`{base_topic}/snapshot`

Payload is a JSON object with the current state.
Messages are published:
- at startup
- every `publish_interval`
- only if the state has changed

### Snapshot payload

Example:

```json
{
  "enabled": true,
  "temperature_setpoint": 22,
  "temperature_setpoint_min": 16,
  "temperature_setpoint_max": 28,
  "mode": "auto",
  "fan_speed": "auto",
  "ambient_temperature": 21
}
```

### Requesting a snapshot

Publish a message to topic `{base_topic}/get/snapshot` to trigger a snapshot publish.

### Writable attributes

| Attribute | Type | Example |
|---------|------|--------|
| `enabled` | bool | `true` |
| `setpoint` | number | `22.5` |
| `min_setpoint` | number | `16` |
| `max_setpoint` | number | `28` |
| `mode` | string | `"heat"` |
| `fan_speed` | string | `"high"` |

To update an attribute, publish to `{base_topic}/set/{attribute}` and add the target value under the `value` field of the message payload. No other fields than `value` are allowed.

Payload format is always:
```json
{ "value": <value> }
```

### Examples

Assuming the broker is running on localhost:1883, and using `mosquitto_pub` :

```sh
mosquitto_pub -h localhost -p 1883 -t "thermocktat/my-thermocktat/set/enabled" -m '{"value":true}'
mosquitto_pub -h localhost -p 1883 -t "thermocktat/my-thermocktat/set/setpoint" -m '{"value":24}'
mosquitto_pub -h localhost -p 1883 -t "thermocktat/my-thermocktat/set/mode" -m '{"value":"heat"}'
```

### Error handling

Invalid commands are ignored:
- unknown attributes
- invalid JSON
- missing `value`
- invalid enum values (mode, fan_speed)

No error messages are published back to MQTT.
