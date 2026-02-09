# Thermocktat integration tests

This is a small python project to run integration tests and verify that thermocktat controllers work as they must from a user perspective.

## Setup

```sh
# Create python virtual environment and install dependencies 
uv sync
```

## Running tests

```sh
# Build binary 
mkdir .bin
go build -o .bin/thermocktat ../cmd/thermocktat

# Run tests
uv run pytest
```

**Note:** for `test_mqtt_control.py`, a mqtt broker is required.
