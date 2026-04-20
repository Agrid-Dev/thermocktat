# BACnet controller

## Configuration

Example (add under `controllers.bacnet` in your app config):

```yaml
controllers:
  bacnet:
    enabled: true
    addr: "0.0.0.0:47808"    # BACnet/IP UDP listen address
    device_instance: 1        # BACnet device instance number (0..4194303)
    sync_interval: 0.5s       # Retained for parity; currently unused
```

Environment variables:

| Variable | Description |
|---|---|
| `TMK_CONTROLLER` | Set to `bacnet` to start only the BACnet controller |
| `TMK_ADDR` | Override `addr` (e.g. `127.0.0.1:47808`) |
| `TMK_CONTROLLERS_BACNET_DEVICE_INSTANCE` | Override device instance number |

## BACnet interface

The controller implements BACnet/IP over UDP and handles three services:

- **Who-Is / I-Am** — Responds to Who-Is broadcasts with an I-Am containing the configured device instance.
- **ReadProperty** — Returns `PresentValue` (property 85) for all supported objects.
- **WriteProperty** — Sets `PresentValue` for writable objects. Returns SimpleACK on success, Error on failure.

Only `PresentValue` (property ID 85) is supported. Requests for other properties receive an Error response.

## Object mapping

| BACnet Object | Instance | Variable | Access |
|---|---:|---|---|
| Analog Input (0) | 0 | `ambient_temperature` | Read-only |
| Analog Value (2) | 0 | `temperature_setpoint` | Read / Write |
| Analog Value (2) | 1 | `temperature_setpoint_min` | Read / Write |
| Analog Value (2) | 2 | `temperature_setpoint_max` | Read / Write |
| Analog Value (2) | 3 | `fault_code` | Read / Write |
| Binary Value (5) | 0 | `enabled` | Read / Write |
| Multi-State Value (19) | 0 | `mode` | Read / Write |
| Multi-State Value (19) | 1 | `fan_speed` | Read / Write |

### Value encoding

- **Temperatures** (AI:0, AV:0, AV:1, AV:2): IEEE 754 float32 in degrees Celsius.
- **Fault Code** (AV:3): integer transported as float32 (truncated to int on write).
- **Enabled** (BV:0): `1.0` = active, `0.0` = inactive.
- **Mode** (MSV:0): `1` = heat, `2` = cool, `3` = fan, `4` = auto.
- **Fan Speed** (MSV:1): `1` = auto, `2` = low, `3` = medium, `4` = high.

## Error handling

- Reading or writing an unknown object returns `ErrorClassObject` / `ErrorCodeUnknownObject`.
- Writing to a read-only object (Analog Input) returns `ErrorClassService` / `ErrorCodeServiceRequestDenied`.
- Requesting a property other than `PresentValue` returns `ErrorClassService` / `ErrorCodeServiceRequestDenied`.

## Known library issues

This controller uses `github.com/ulbios/bacnet`. Two bugs in the library are worked around in the controller code:

1. **`IAmObjects` ignores the device instance parameter** — hardcodes instance 321. The controller builds I-Am objects manually.
2. **`DecObjectIdentifier` bit-shift error** — uses `>> 20` instead of `>> 22`, causing object types to decode as `type * 4`. The controller implements its own decoders.

## See also

- HTTP controller docs: [../http/README.md](../http/README.md)
- MQTT controller docs: [../mqtt/README.md](../mqtt/README.md)
- Modbus controller docs: [../modbus/README.md](../modbus/README.md)
