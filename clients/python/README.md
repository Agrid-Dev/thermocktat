# Thermocktat client

A http client for [thermocktat](https://github.com/Agrid-Dev/thermocktat).

Thermocktat = mock + thermostat

It emulates a thermostat device with realistic controls and temperature simulation, and supports many control protocols (http, mqtt, modbus, bacnet, knx...).

It is primarily design to test and demo Building Management Systems (BMS) software.

This lib is a lightweight wrapper around its http API.

See the source repo for details.

## Installation

```sh
uv add thermocktat-client        # or: pip install thermocktat-client
```

## Usage

### Sync

```python
from thermocktat_client import ThermocktatSync

with ThermocktatSync.connect("http://localhost:8080") as tmk:
    print(tmk.snapshot.temperature_setpoint)
    tmk.set_temperature_setpoint(23.5)
    tmk.set_mode("cool")
    print(tmk.snapshot.mode)
```

### Async

```python
import asyncio
from thermocktat_client import ThermocktatAsync

async def main():
    async with await ThermocktatAsync.connect("http://localhost:8080") as tmk:
        print(tmk.snapshot.temperature_setpoint)
        await tmk.set_temperature_setpoint(23.5)
        await tmk.set_mode("cool")

asyncio.run(main())
```

### Pure construction (no network on init)

`__init__` is side-effect-free. Call `.sync()` (or use `.connect(...)`, which combines construct + sync) before reading `.snapshot`, otherwise properties raise `NotSyncedError`.

```python
tmk = ThermocktatSync("http://localhost:8080")
tmk.sync()
print(tmk.snapshot)
```

### Injecting a custom httpx client

Useful for auth, retries, or sharing a connection pool. The client is the caller's to close.

```python
from httpx import Client
from thermocktat_client import ThermocktatSync

external = Client(headers={"Authorization": "Bearer ..."})
tmk = ThermocktatSync.connect("http://...", client=external)
# external.close() when you're done
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).
