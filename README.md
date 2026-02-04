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


## Regulation - ambient temperature simulation

The regulation of ambient temperature is simulated using a [PID regulator](https://en.wikipedia.org/wiki/Proportional%E2%80%93integral%E2%80%93derivative_controller) and hysteresis (see diagram below).


<p align="center">
  <img src="assets/thermocktat_regulation.png" alt="Thermocktat Regulation Diagram"/>
</p>

Regulation simulates the effect of heating or cooling systems controlled by the thermostat that will actually heat and cool the room in order to reach the desired temperature (setpoint).

This example is for the heating mode. If the ambient temperature is above setpoint, or below within a hysteresis range (- `TargetHysteresis`, 1°C in this example), heating is not triggered. When it is lower with a difference greater than the target hysteresis, heating start until the target temperature is reached. The target is the setpoint temperature plus the target hysteresis.

In `auto` mode, a second hysteresis `ModeChangeHysteresis` (greater than `TargetHysteresis`) can trigger switching regulation direction between cooling and heating. For example, if the `TargetHysteresis` is 1 and the `ModeChangeHysteresis` is 2 (default values):

- if temperature setpoint is 20, and ambient temperature is above 22 (`setpoint + ModeChangeHysteresis`), regulation will switch to cooling, and cool until 19 (`setpoint - TargetHysteresis`);
- if temperature setpoint is 20, and ambient temperature is below 18 (`setpoint - ModeChangeHysteresis`), regulation will switch to heating, and cool until 21 (`setpoint + TargetHysteresis`).

Regulation params can be set in the `config.yaml` file (see `config.example.yaml`). Regulation can also be disabled (in this case, ambient temperature will remain constant).

## API Documentation
- [HTTP Controller API](internal/controllers/http/README.md)
- [MQTT Controller API](internal/controllers/mqtt/README.md)
- [Modbus Controller API](internal/controllers/modbus/README.md)
- [BACnet Controller API](internal/controllers/bacnet/README.md) (Coming soon)



## Configuration

Thermocktat can be configure from file (see `config.example.yaml`).

Usage:
```sh
go run ./cmd/thermocktat -config config.yaml
```

Configuration can also be passed using environment variables with the `TMK_` prefix. Environment variables have priority over config file.

If no config is provided, default values will be used and server will run on port 8080.

For each controller, the `addr` field is in the format `host:port` (`host` will be `localhost` by default). For most controllers, it is used to set the url that the server will expose. For `mqtt`, `addr` is the address of the broker.

Example:

```sh
TMK_CONTROLLER=http \
TMK_ADDR=:8080 \
go run ./cmd/thermocktat
```

## Docker Examples

```sh
# Run with default params
docker run -p 8080:8080 thermocktat

# Set controller and address using environment variables
docker run --rm -e TMK_CONTROLLER=http -e TMK_ADDR=:8080 -p 8080:8080 thermocktat-dev
docker run --rm -e TMK_CONTROLLER=mqtt -e TMK_ADDR=tcp://host.docker.internal:1883 -e TMK_DEVICE_ID=my-thermocktat thermocktat
docker run --rm -e TMK_CONTROLLER=modbus -e TMK_ADDR=0.0.0.0:1502 -e TMK_DEVICE_ID=my-thermocktat-2 -p 1502:1502 thermocktat

# Run with a config file mounted as a volume
docker run -v $(pwd)/config.yaml:/config.yaml -p 8080:8080 thermocktat -config /config.yaml
```
