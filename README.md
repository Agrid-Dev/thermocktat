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

## API

Currently has only an `http` api. More control protocols to come.

`GET /v1`

- Description: Retrieve the current attributes of the thermostat.
- Method: GET
- Example Request: `GET /v1`

`POST /v1/:attribute`

- Description: Update a specific attribute of the thermostat.
- Method: POST
- Request Body: JSON object with the format `{"value": <value>}`
- Example Request: `POST /v1/enabled {"value": true}`

`GET /healthz`

Returns "ok" if server is running.

### Examples
- Enable the thermostat:
  ```
  POST /v1/enabled
  {
    "value": true
  }
  ```

- Set the temperature setpoint:
  ```
  POST /v1/setpoint
  {
    "value": 21
  }
  ```

- Change the operating mode:
  ```
  POST /v1/mode
  {
    "value": "heat"
  }
  ```

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
