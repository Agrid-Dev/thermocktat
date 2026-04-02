# KNX controller

## Configuration

Example (add under `controllers.knx` in your app config):

```yaml
controllers:
  knx:
    enabled: true
    addr: "0.0.0.0:3671"    # KNXnet/IP UDP listen address
    publish_interval: 10s    # how often to push state changes to connected client
    ga_main: 1               # group address main group (0–31)
    ga_middle: 0             # group address middle group (0–7)
```

Environment variables:

| Variable | Description |
|---|---|
| `TMK_CONTROLLER` | Set to `knx` to start only the KNX controller |
| `TMK_ADDR` | Override `addr` (e.g. `127.0.0.1:3671`) |
| `TMK_CONTROLLERS_KNX_PUBLISH_INTERVAL` | Override publish interval (e.g. `5s`) |
| `TMK_CONTROLLERS_KNX_GA_MAIN` | Override group address main group |
| `TMK_CONTROLLERS_KNX_GA_MIDDLE` | Override group address middle group |

## KNXnet/IP interface

The controller implements a **KNXnet/IP tunneling server** over UDP. It supports a single concurrent tunnel client (e.g. xknx, ETS).

### Supported services

| Service | Code | Direction |
|---|---|---|
| CONNECT_REQUEST / RESPONSE | 0x0205 / 0x0206 | client to server |
| CONNECTIONSTATE_REQUEST / RESPONSE | 0x0207 / 0x0208 | client to server |
| DISCONNECT_REQUEST / RESPONSE | 0x0209 / 0x020A | client to server |
| TUNNELING_REQUEST / ACK | 0x0420 / 0x0421 | bidirectional |

### Connection behavior

- **Single-client**: a second CONNECT_REQUEST is rejected with status `E_NO_MORE_CONNECTIONS` (0x24).
- **NAT support**: HPAI 0.0.0.0:0 is handled by using the UDP source address (xknx default).
- **Heartbeat**: connection is dropped after 120s without a CONNECTIONSTATE_REQUEST.

### CEMI / group value handling

- **GroupValueRead**: server responds with GroupValueResponse containing the current value.
- **GroupValueWrite**: server updates the thermostat and acknowledges.
- **State push**: server periodically checks for state changes and sends GroupValueWrite telegrams to the connected client (push-based, like a real KNX device).

## Group address mapping

Sub-addresses are hardcoded. The main and middle groups are configurable.

Default group addresses (main=1, middle=0):

| Group Address | Sub | Variable | DPT | Access |
|---|---:|---|---|---|
| 1/0/0 | 0 | `enabled` | 1.001 (Switch) | Read / Write |
| 1/0/1 | 1 | `temperature_setpoint` | 9.001 (2-byte float) | Read / Write |
| 1/0/2 | 2 | `temperature_setpoint_min` | 9.001 | Read / Write |
| 1/0/3 | 3 | `temperature_setpoint_max` | 9.001 | Read / Write |
| 1/0/4 | 4 | `ambient_temperature` | 9.001 | Read-only |
| 1/0/5 | 5 | `mode` | 20.102 (HVAC Mode) | Read / Write |
| 1/0/6 | 6 | `fan_speed` | 5.010 (1-byte unsigned) | Read / Write |

### Value encoding

- **Temperatures** (sub 1–4): KNX 2-byte float (DPT 9.001). Encoding: `0.01 * mantissa * 2^exponent`.
- **Enabled** (sub 0): 1-bit compact encoding in APCI low bits. `1` = on, `0` = off.
- **Mode** (sub 5): 1-byte unsigned. `1` = heat, `2` = cool, `3` = fan, `4` = auto.
- **Fan Speed** (sub 6): 1-byte unsigned. `1` = auto, `2` = low, `3` = medium, `4` = high.

## Not supported

- Multi-client tunneling
- KNX routing (multicast)
- SEARCH_REQUEST / DESCRIPTION_REQUEST (discovery)
- KNXnet/IP Secure

## See also

- HTTP controller docs: [../http/README.md](../http/README.md)
- MQTT controller docs: [../mqtt/README.md](../mqtt/README.md)
- Modbus controller docs: [../modbus/README.md](../modbus/README.md)
- BACnet controller docs: [../bacnet/README.md](../bacnet/README.md)
