# Modbus controller

## Configuration

Example (add under `controllers.modbus` in your app config):

```yaml
controllers:
  modbus:
    enabled: true
    listen_addr: "127.0.0.1:1502"   # Modbus TCP listen address
    unit_id: 1                      # Modbus Unit/Slave ID (integer 1..247)
    sync_interval: 1s               # Optional: copy service snapshot into Modbus memory periodically; set 0 to disable
```

Important notes:
- `unit_id` is numeric (Modbus Unit / Slave ID). This is not the same as the human `device_id` string used by other controllers.
- `sync_interval` controls how often the controller writes the current thermostat snapshot into the Modbus server memory so Modbus read requests see updates that may have come from outside Modbus. If all changes always arrive via Modbus, you can set `sync_interval: 0` and skip periodic syncing.

## Modbus interface

This controller exposes thermostat state using standard Modbus data types:

- Coils (bit access)
  - Coil 0: `enabled` (read/write)

- Holding Registers (read/write, 16-bit each)
  - HR 0: `temperature_setpoint` — encoded as signed 16-bit integer = value * 100
  - HR 1: `temperature_setpoint_min` — encoded as signed 16-bit integer = value * 100
  - HR 2: `temperature_setpoint_max` — encoded as signed 16-bit integer = value * 100
  - HR 3: `mode` — encoded as integer (use values from `thermostat.Mode`, e.g. `1 = heat`, `2 = cool`, etc.)
  - HR 4: `fan_speed` — encoded as integer (use values from `thermostat.FanSpeed`)

- Input Registers (read-only, 16-bit each)
  - IR 0: `ambient_temperature` — encoded as signed 16-bit integer = value * 100

Supported Modbus function codes:
- 0x01 Read Coils
- 0x05 Write Single Coil
- 0x03 Read Holding Registers
- 0x06 Write Single Register
- 0x10 Write Multiple Registers (supported for HR 0..4)
- 0x04 Read Input Registers

Addressing convention:
- On-the-wire PDU addresses are zero-based (the controller uses 0-based addresses in code and tests).
- Many user-facing tools and documentation use 1-based "reference numbers" such as `40001` for holding register 0. See the table below for both representations.

## Register / Coil mapping table

Variable                        | Type          | PDU address (0-based) | Human reference (common) | Encoding / Notes |
|---------------------------------|---------------|-----------------------:|--------------------------:|------------------|
| enabled                         | Coil          | Coil 0                 | 00001                    | Coil: 0 = OFF, 1 = ON (Write Single Coil 0x0000 = OFF, 0xFF00 = ON) |
| temperature_setpoint            | HR (holding)  | HR 0                   | 40001                    | signed int16, scaled by 100 (value * 100). Example: 22.50 -> 2250 |
| temperature_setpoint_min        | HR (holding)  | HR 1                   | 40002                    | signed int16, scaled by 100 |
| temperature_setpoint_max        | HR (holding)  | HR 2                   | 40003                    | signed int16, scaled by 100 |
| mode                            | HR (holding)  | HR 3                   | 40004                    | uint16 enum corresponding to `thermostat.Mode` values |
| fan_speed                       | HR (holding)  | HR 4                   | 40005                    | uint16 enum corresponding to `thermostat.FanSpeed` values |
| ambient_temperature (read-only) | IR (input)    | IR 0                   | 30001                    | signed int16, scaled by 100


Scaling reminder:
- Temperatures are encoded as signed 16-bit integers representing the temperature multiplied by 100 (two decimal places). This keeps values compact in a single 16-bit register.
  - Example: to represent 21.25°C store 2125 (0x084D). To decode, treat value as signed int16 then divide by 100.0.
- Mode and fan speed are stored as numeric enums (the same integers used in the Go `thermostat` package).

## Error handling / validation

- Writes are applied synchronously using registered mbserver handlers. If the underlying thermostat service rejects a value (for example, invalid setpoint, invalid enum), the controller returns a Modbus exception:
  - `Illegal Data Value` (Modbus exception 3) for invalid values.
  - `Illegal Data Address` (Modbus exception 2) for unsupported addresses.
- Read requests return current values from the service (the controller periodically syncs the service snapshot into Modbus memory; see `sync_interval`).


## See also

- HTTP controller docs: [../http/README.md](../http/README.md)
- MQTT controller docs: [../mqtt/README.md](../mqtt/README.md)
