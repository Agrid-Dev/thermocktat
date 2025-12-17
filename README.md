# Thermocktat

A lightweight thermostat emulator, primarily designed for BMS software testing (Building Management Systems).

## Attributes

| Name           | Type    | Default   | Comment                                      |
|----------------|---------|-----------|----------------------------------------------|
| ambient_temperature    | float   | 21.0      | Current temperature reading.      |
| setpoint       | float   | 22.0      | Target temperature. Must be between `min_setpoint` and `max_setpoint`.        |
| mode           | string  | "auto"    | Operating mode: "auto", "heat", "cool", "fan". |
| fan_speed      | string  | "medium"  | Fan speed setting: "auto", "low", "medium", or "high".  |
| enabled          | boolean | true      | Indicates if the thermostat is powered (on/off).   |
| min_setpoint       | float   | 16.0      | `setpoint` lower bound.    |
| max_setpoint       | float   | 28.0      | `setpoint` upper bound.   |



## API Documentation
- [HTTP Controller API](internal/controllers/http/README.md)
- [MQTT Controller API](internal/controllers/mqtt/README.md)
- [Modbus Controller API](internal/controllers/modbus/README.md) (Coming soon)
- [BACnet Controller API](internal/controllers/bacnet/README.md) (Coming soon)



## Configuration

Can dynamically populate initial values and set server configuration with a `config.yaml` file (see `config.example.yaml`).

Usage:
```sh
go run ./cmd/thermocktat -config config.yaml
```

If no config is provided, default values will be used and server will run on port 8080.

Can also run with environment variables (which have priority over config) :
```sh
THERMOCKSTAT_HTTP_ADDR=:3001 go run ./cmd/thermocktat
# or
PORT=3001 go run ./cmd/thermocktat
```

## Docker Examples

```sh
docker run -p 8080:8080 thermocktat
docker run -v $(pwd)/config.yaml:/config.yaml -p 8080:8080 thermocktat -config /config.yaml
```
