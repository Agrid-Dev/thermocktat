# Thermocktat

A lightweight thermostat emulator, primarily designed for BMS software testing (Building Management Systems).

<p align="center">
  <img src="https://github.com/user-attachments/assets/60c5e587-035c-4f38-b396-9695343a1a75" alt="A cool dino just for fun"/>
</p>


## Attributes

| Name           | Type    | Default   | Comment                                      |
|----------------|---------|-----------|----------------------------------------------|
| ambient_temperature    | float   | 21.0      | Current temperature reading.      |
| setpoint_temperature  | float   | 22.0      | Target temperature. Must be between `setpoint_temperature_min` and `setpoint_temperature_max`. |
| mode           | string  | "auto"    | Operating mode: `auto \| heat \| cool \| fan`. |
| fan_speed      | string  | "medium"  | Fan speed setting: `auto \| low \| medium \| high`.  |
| enabled          | boolean | true      | Indicates if the thermostat is powered (on/off).   |
| setpoint_temperature_min  | float   | 16.0      | `setpoint` lower bound.    |
| setpoint_temperature_max  | float   | 28.0      | `setpoint` upper bound.   |



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
