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
    register_count: 1               # 1 = 16-bit mode (default), 2 = 32-bit mode (IEEE 754 float32)
```

Important notes:
- `unit_id` is numeric (Modbus Unit / Slave ID). This is not the same as the human `device_id` string used by other controllers.
- `sync_interval` controls how often the controller writes the current thermostat snapshot into the Modbus server memory so Modbus read requests see updates that may have come from outside Modbus. If all changes always arrive via Modbus, you can set `sync_interval: 0` and skip periodic syncing.
- `register_count` controls how temperature float values are encoded:
  - `1` (default): 16-bit mode. Each temperature is stored as a signed int16 scaled by 100 in a single register.
  - `2`: 32-bit mode. Each temperature is stored as an IEEE 754 float32 across two consecutive 16-bit registers (big-endian word order).

## Modbus interface

This controller exposes thermostat state using standard Modbus data types:

- Coils (bit access)
  - Coil 0: `enabled` (read/write)

- Holding Registers (read/write)
  - HR 0–1: `temperature_setpoint`
  - HR 2–3: `temperature_setpoint_min`
  - HR 4–5: `temperature_setpoint_max`
  - HR 6: `mode` — uint16 enum (e.g. `1 = heat`, `2 = cool`, etc.)
  - HR 8: `fan_speed` — uint16 enum

- Input Registers (read-only)
  - IR 0–1: `ambient_temperature`

Register addresses are spaced by 2 so each temperature field can occupy either 1 register (16-bit mode) or 2 consecutive registers (32-bit mode) without changing the base address layout.

### 16-bit mode (`register_count: 1`, default)

Temperatures are encoded as signed 16-bit integers = value * 100 in the first register of each pair. The second register in each pair is unused (reads as 0). Enum fields (mode, fan_speed) occupy a single register at their base address.

Write single register (function 6) works for all fields. Write multiple registers (function 16) also works.

### 32-bit mode (`register_count: 2`)

Temperatures are encoded as IEEE 754 float32 split across two consecutive registers (big-endian word order: high word first). Enum fields (mode, fan_speed) are still single uint16 registers.

Write single register (function 6) only works for enum fields (mode at HR 6, fan_speed at HR 8). Temperature writes must use write multiple registers (function 16) to write both registers of the pair.

Supported Modbus function codes:
- 0x01 Read Coils
- 0x05 Write Single Coil
- 0x03 Read Holding Registers
- 0x06 Write Single Register
- 0x10 Write Multiple Registers
- 0x04 Read Input Registers

Addressing convention:
- On-the-wire PDU addresses are zero-based (the controller uses 0-based addresses in code and tests).
- Many user-facing tools and documentation use 1-based "reference numbers" such as `40001` for holding register 0. See the table below for both representations.

## Register / Coil mapping table

| Variable                        | Type          | PDU address (0-based) | Human reference (common) | Encoding / Notes |
|---------------------------------|---------------|-----------------------:|--------------------------:|------------------|
| enabled                         | Coil          | Coil 0                 | 00001                    | Coil: 0 = OFF, 1 = ON (Write Single Coil 0x0000 = OFF, 0xFF00 = ON) |
| temperature_setpoint            | HR (holding)  | HR 0–1                 | 40001–40002              | 16-bit: signed int16 * 100 in HR 0. 32-bit: float32 across HR 0–1 |
| temperature_setpoint_min        | HR (holding)  | HR 2–3                 | 40003–40004              | 16-bit: signed int16 * 100 in HR 2. 32-bit: float32 across HR 2–3 |
| temperature_setpoint_max        | HR (holding)  | HR 4–5                 | 40005–40006              | 16-bit: signed int16 * 100 in HR 4. 32-bit: float32 across HR 4–5 |
| mode                            | HR (holding)  | HR 6                   | 40007                    | uint16 enum corresponding to `thermostat.Mode` values |
| fan_speed                       | HR (holding)  | HR 8                   | 40009                    | uint16 enum corresponding to `thermostat.FanSpeed` values |
| ambient_temperature (read-only) | IR (input)    | IR 0–1                 | 30001–30002              | 16-bit: signed int16 * 100 in IR 0. 32-bit: float32 across IR 0–1 |

Scaling reminder (16-bit mode):
- Temperatures are encoded as signed 16-bit integers representing the temperature multiplied by 100 (two decimal places). This keeps values compact in a single 16-bit register.
  - Example: to represent 21.25°C store 2125 (0x084D). To decode, treat value as signed int16 then divide by 100.0.

Encoding reminder (32-bit mode):
- Temperatures are encoded as IEEE 754 float32 split across two registers (big-endian word order).
  - Example: 22.5°C → `math.Float32bits(22.5)` = `0x41B40000` → HR 0 = `0x41B4`, HR 1 = `0x0000`.

- Mode and fan speed are stored as numeric enums (the same integers used in the Go `thermostat` package) in both modes.

## Error handling / validation

- Writes are applied synchronously using registered mbserver handlers. If the underlying thermostat service rejects a value (for example, invalid setpoint, invalid enum), the controller returns a Modbus exception:
  - `Illegal Data Value` (Modbus exception 3) for invalid values.
  - `Illegal Data Address` (Modbus exception 2) for unsupported addresses, or when attempting to write a single temperature register in 32-bit mode.
- Read requests return current values from the service (the controller periodically syncs the service snapshot into Modbus memory; see `sync_interval`).


## See also

- HTTP controller docs: [../http/README.md](../http/README.md)
- MQTT controller docs: [../mqtt/README.md](../mqtt/README.md)
