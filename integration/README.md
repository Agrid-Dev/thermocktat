# Thermocktat integration tests

This is a small python project to run integration tests and verify that thermocktat controllers work as they must from a user perspective.

## Setup

```sh
# Create python virtual environment and install dependencies 
uv sync
```

## Running tests

```sh
# Build binary (CGO_ENABLED=0 produces a static binary required for Docker-based BACnet tests)
mkdir -p .bin
CGO_ENABLED=0 go build -o .bin/thermocktat ../cmd/thermocktat

# Run tests
uv run pytest
```

**Note:**
- For `test_mqtt_control.py`, a MQTT broker is required (automatically started in CI).
- For `test_bacnet_control.py`, Docker is required. The tests will automatically:
  - Build a minimal Docker image from the current binary (cross-compiled for Linux if needed)
  - Run the container with UDP port `47808` published to the host
  - Stop and remove the container after tests complete
