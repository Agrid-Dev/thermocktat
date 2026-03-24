# Thermocktat

This repo is a thermostat emulator (thermostat + mock = thermocktat, because we like fun also).

Its primary usage is to serve as a test or simulation device for Building Management Systems (BMS) applications. It simulates a typical room thermostat such as for a hotel room, with basic controls (`enabled`, `temperature_setpoint`, `mode`, `fan_speed`...), regulation by the HVAC system it’s supposed to control, and heat losses to the outside through room walls.

It is implemented in go and open source.

## Project structure

It is made of one main thermostat application (internal/thermostat) with several controllers (modbus, bacnet, http, mqtt) in internal/controllers. See README files within each controller for api documentation.

The `cmd` directory contains the code to configure and run it.

## Configuration

Thermocktat has many configurable parameters (for controllers, initial state, regulation params...). It can also run with full default config if nothing is provided. There are 3 ways to pass config, in order of increasing priority:

- default config `cmd/app/config_defaults.yaml` is a yaml file embedded into the build that contains all default values. It serves also as a reference for building a
- `config.yaml` file, with parameters overriding the defaults. Run with `go run ./cmd/thermocktat -config config.yaml` ;
- last, environment variables have the top-level priority, they all have the `TMK_` prefix and should be upper snake case. See `cmd/app/config.go`.

## Architecture

Controllers (modbus, bacnet, http, mqtt) depend on the `ThermostatService` interface defined in `internal/ports/thermostat.go`, not on the concrete thermostat. This is the key decoupling boundary.

## Running locally

### Build

```sh
mkdir -p .bin
CGO_ENABLED=0 go build -o .bin/thermocktat ./cmd/thermocktat
```

### Unit tests

```sh
go test -race ./...
```

### Integration tests

The `integration/` directory is a separate Python project (uv + pytest) that tests each controller protocol end-to-end against a built binary.

```sh
cd integration
uv sync
uv run pytest
```

Some tests require external services: MQTT broker for `test_mqtt_control.py`, Docker for `test_bacnet_control.py`.

## Code quality

### Go

- `gofmt -w .` — formatting
- `goimports -w .` — import ordering
- `go vet ./...` — static analysis
- `gopls check` — linting

### Python (integration tests)

- `ruff check .` — linting
- `ruff format --check .` — formatting
- `ty check` — type checking

## Deployment and CI

The CI runs on Github Action (see `.github/workflows`).

The app is primarily distributed as a docker image (see `Dockerfile`). It must stay lightweight because we want to run typically 100+ instances on a machine to simulate a real building.

The CI workflow `ci.yaml` enforces all Go quality checks, unit tests, and integration tests. The release workflow `release-image.yaml` releases an image to registry ghcr.io whenever a new release is pushed.
