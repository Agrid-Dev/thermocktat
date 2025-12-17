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
