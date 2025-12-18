# HTTP controller

## Configuration

Example:

```yaml
controllers:
  http:
    enabled: true
    addr: ":8080"
```

## API

### Endpoints


| Attribute                  | Method | Path                              | Example Payload     |
|----------------------------|--------|-----------------------------------|---------------------|
| Health Check               | GET    | /healthz                          | N/A                 |
| Full Snapshot              | GET    | /v1                               | N/A                 |
| Enabled                    | POST   | /v1/enabled                       | {"value": true}     |
| Temperature Setpoint       | POST   | /v1/temperature_setpoint          | {"value": 22.5}     |
| Temperature Setpoint Min   | POST   | /v1/temperature_setpoint_min      | {"value": 16.0}     |
| Temperature Setpoint Max   | POST   | /v1/temperature_setpoint_max      | {"value": 28.0}     |
| Mode                       | POST   | /v1/mode                          | {"value": "cool"}   |
| Fan Speed                  | POST   | /v1/fan_speed                     | {"value": "high"}   |

`GET /v1`

- Description: Retrieve a snapshot with the current attributes of the thermostat.
- Method: GET
- Example Request: `GET /v1`

Snapshot response:
```json
{
  "device_id": "my-thermocktat",
  "enabled": true,
  "temperature_setpoint": 22,
  "temperature_setpoint_min": 16,
  "temperature_setpoint_max": 28,
  "mode": "auto",
  "fan_speed": "auto",
  "ambient_temperature": 21
}
```

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
