import asyncio

import pytest
from xknx import XKNX
from xknx.core import ValueReader
from xknx.dpt import DPTArray, DPTBinary, DPTTemperature
from xknx.io import ConnectionConfig, ConnectionType
from xknx.telegram import GroupAddress, Telegram
from xknx.telegram.apci import GroupValueResponse, GroupValueWrite

# Default group addresses (must match config_defaults.yaml)
GA_ENABLED = GroupAddress("1/0/0")
GA_SETPOINT = GroupAddress("1/0/1")
GA_SETPOINT_MIN = GroupAddress("1/0/2")
GA_SETPOINT_MAX = GroupAddress("1/0/3")
GA_AMBIENT_TEMPERATURE = GroupAddress("1/0/4")
GA_MODE = GroupAddress("1/0/5")
GA_FAN_SPEED = GroupAddress("1/0/6")
GA_FAULT_CODE = GroupAddress("1/0/7")

# Thermostat enum values (same as Go internal enums, same as Modbus)
MODE_HEAT = 1
MODE_COOL = 2
MODE_FAN = 3
MODE_AUTO = 4

FAN_AUTO = 1
FAN_LOW = 2
FAN_MEDIUM = 3
FAN_HIGH = 4

KNX_PORT = 3671


def _connection_config() -> ConnectionConfig:
    return ConnectionConfig(
        connection_type=ConnectionType.TUNNELING,
        gateway_ip="127.0.0.1",
        gateway_port=KNX_PORT,
        auto_reconnect=False,
    )


async def _read_ga(xknx: XKNX, ga: GroupAddress) -> DPTBinary | DPTArray:
    """Send GroupValueRead and return the response payload."""
    reader = ValueReader(xknx, ga, timeout_in_seconds=5)
    telegram = await reader.read()
    assert telegram is not None, f"No response for GroupValueRead on {ga}"
    payload = telegram.payload
    assert isinstance(payload, (GroupValueResponse, GroupValueWrite)), (
        f"Expected GroupValueResponse or GroupValueWrite, got {type(payload)}"
    )
    return payload.value


async def _write_ga(xknx: XKNX, ga: GroupAddress, payload: DPTBinary | DPTArray):
    """Send GroupValueWrite."""
    telegram = Telegram(
        destination_address=ga,
        payload=GroupValueWrite(payload),
    )
    await xknx.telegrams.put(telegram)
    # Give the server time to process the write
    await asyncio.sleep(0.2)


def _decode_dpt9(payload: DPTArray) -> float:
    """Decode a DPT 9.001 (2-byte KNX float) payload."""
    return DPTTemperature.from_knx(payload)


def _encode_dpt9(value: float) -> DPTArray:
    """Encode a float as DPT 9.001 (2-byte KNX float)."""
    return DPTTemperature.to_knx(value)


# --- Connection test ---


@pytest.mark.asyncio
async def test_connect_disconnect(knx_tmk_application):
    """xknx can connect to and disconnect from the KNX/IP tunneling server."""
    xknx = XKNX(connection_config=_connection_config())
    await xknx.start()
    try:
        assert xknx.connection_manager.connected.is_set()
    finally:
        await xknx.stop()


# --- Read tests ---


@pytest.mark.asyncio
async def test_read_enabled(knx_tmk_application):
    """Read the enabled state (DPT 1.001) — default is true (1)."""
    xknx = XKNX(connection_config=_connection_config())
    await xknx.start()
    try:
        payload = await _read_ga(xknx, GA_ENABLED)
        assert isinstance(payload, DPTBinary)
        assert payload.value == 1
    finally:
        await xknx.stop()


@pytest.mark.asyncio
async def test_read_setpoint(knx_tmk_application):
    """Read temperature setpoint (DPT 9.001) — default is 22.0."""
    xknx = XKNX(connection_config=_connection_config())
    await xknx.start()
    try:
        payload = await _read_ga(xknx, GA_SETPOINT)
        assert isinstance(payload, DPTArray)
        value = _decode_dpt9(payload)
        assert abs(value - 22.0) < 0.5
    finally:
        await xknx.stop()


@pytest.mark.asyncio
async def test_read_setpoint_min(knx_tmk_application):
    """Read setpoint min (DPT 9.001) — default is 16.0."""
    xknx = XKNX(connection_config=_connection_config())
    await xknx.start()
    try:
        payload = await _read_ga(xknx, GA_SETPOINT_MIN)
        assert isinstance(payload, DPTArray)
        value = _decode_dpt9(payload)
        assert abs(value - 16.0) < 0.5
    finally:
        await xknx.stop()


@pytest.mark.asyncio
async def test_read_setpoint_max(knx_tmk_application):
    """Read setpoint max (DPT 9.001) — default is 28.0."""
    xknx = XKNX(connection_config=_connection_config())
    await xknx.start()
    try:
        payload = await _read_ga(xknx, GA_SETPOINT_MAX)
        assert isinstance(payload, DPTArray)
        value = _decode_dpt9(payload)
        assert abs(value - 28.0) < 0.5
    finally:
        await xknx.stop()


@pytest.mark.asyncio
async def test_read_ambient_temperature(knx_tmk_application):
    """Read ambient temperature (DPT 9.001) — default is 21.0."""
    xknx = XKNX(connection_config=_connection_config())
    await xknx.start()
    try:
        payload = await _read_ga(xknx, GA_AMBIENT_TEMPERATURE)
        assert isinstance(payload, DPTArray)
        value = _decode_dpt9(payload)
        assert abs(value - 21.0) < 0.5
    finally:
        await xknx.stop()


@pytest.mark.asyncio
async def test_read_mode(knx_tmk_application):
    """Read HVAC mode (DPT 20.102) — default is auto (4)."""
    xknx = XKNX(connection_config=_connection_config())
    await xknx.start()
    try:
        payload = await _read_ga(xknx, GA_MODE)
        assert isinstance(payload, DPTArray)
        assert payload.value == (MODE_AUTO,)
    finally:
        await xknx.stop()


@pytest.mark.asyncio
async def test_read_fan_speed(knx_tmk_application):
    """Read fan speed (DPT 5.010) — default is auto (1)."""
    xknx = XKNX(connection_config=_connection_config())
    await xknx.start()
    try:
        payload = await _read_ga(xknx, GA_FAN_SPEED)
        assert isinstance(payload, DPTArray)
        assert payload.value == (FAN_AUTO,)
    finally:
        await xknx.stop()


# --- Write tests ---


@pytest.mark.asyncio
@pytest.mark.parametrize("value", [False, True])
async def test_write_enabled(knx_tmk_application, value):
    """Write enabled (DPT 1.001) and read back."""
    xknx = XKNX(connection_config=_connection_config())
    await xknx.start()
    try:
        await _write_ga(xknx, GA_ENABLED, DPTBinary(int(value)))
        payload = await _read_ga(xknx, GA_ENABLED)
        assert isinstance(payload, DPTBinary)
        assert payload.value == int(value)
    finally:
        await xknx.stop()


@pytest.mark.asyncio
async def test_write_setpoint(knx_tmk_application):
    """Write temperature setpoint (DPT 9.001) and read back."""
    xknx = XKNX(connection_config=_connection_config())
    await xknx.start()
    try:
        await _write_ga(xknx, GA_SETPOINT, _encode_dpt9(20.0))
        payload = await _read_ga(xknx, GA_SETPOINT)
        assert isinstance(payload, DPTArray)
        value = _decode_dpt9(payload)
        assert abs(value - 20.0) < 0.5
    finally:
        await xknx.stop()


@pytest.mark.asyncio
async def test_write_setpoint_min(knx_tmk_application):
    """Write setpoint min (DPT 9.001) and read back."""
    xknx = XKNX(connection_config=_connection_config())
    await xknx.start()
    try:
        await _write_ga(xknx, GA_SETPOINT_MIN, _encode_dpt9(18.0))
        payload = await _read_ga(xknx, GA_SETPOINT_MIN)
        assert isinstance(payload, DPTArray)
        value = _decode_dpt9(payload)
        assert abs(value - 18.0) < 0.5
    finally:
        await xknx.stop()


@pytest.mark.asyncio
async def test_write_setpoint_max(knx_tmk_application):
    """Write setpoint max (DPT 9.001) and read back."""
    xknx = XKNX(connection_config=_connection_config())
    await xknx.start()
    try:
        await _write_ga(xknx, GA_SETPOINT_MAX, _encode_dpt9(26.0))
        payload = await _read_ga(xknx, GA_SETPOINT_MAX)
        assert isinstance(payload, DPTArray)
        value = _decode_dpt9(payload)
        assert abs(value - 26.0) < 0.5
    finally:
        await xknx.stop()


@pytest.mark.asyncio
@pytest.mark.parametrize("mode", [MODE_HEAT, MODE_COOL, MODE_FAN, MODE_AUTO])
async def test_write_mode(knx_tmk_application, mode):
    """Write HVAC mode (DPT 20.102) and read back."""
    xknx = XKNX(connection_config=_connection_config())
    await xknx.start()
    try:
        await _write_ga(xknx, GA_MODE, DPTArray(mode))
        payload = await _read_ga(xknx, GA_MODE)
        assert isinstance(payload, DPTArray)
        assert payload.value == (mode,)
    finally:
        await xknx.stop()


@pytest.mark.asyncio
async def test_read_fault_code_default(knx_tmk_application):
    """Read fault_code (DPT 7.001) — default is 0."""
    xknx = XKNX(connection_config=_connection_config())
    await xknx.start()
    try:
        payload = await _read_ga(xknx, GA_FAULT_CODE)
        assert isinstance(payload, DPTArray)
        value = (payload.value[0] << 8) | payload.value[1]
        assert value == 0
    finally:
        await xknx.stop()


@pytest.mark.asyncio
@pytest.mark.parametrize("code", [0, 1, 42, 9999])
async def test_write_fault_code(knx_tmk_application, code):
    """Write fault_code (DPT 7.001) and read back."""
    xknx = XKNX(connection_config=_connection_config())
    await xknx.start()
    try:
        payload = DPTArray(((code >> 8) & 0xFF, code & 0xFF))
        await _write_ga(xknx, GA_FAULT_CODE, payload)
        received = await _read_ga(xknx, GA_FAULT_CODE)
        assert isinstance(received, DPTArray)
        value = (received.value[0] << 8) | received.value[1]
        assert value == code
    finally:
        await xknx.stop()


@pytest.mark.asyncio
@pytest.mark.parametrize("speed", [FAN_AUTO, FAN_LOW, FAN_MEDIUM, FAN_HIGH])
async def test_write_fan_speed(knx_tmk_application, speed):
    """Write fan speed (DPT 5.010) and read back."""
    xknx = XKNX(connection_config=_connection_config())
    await xknx.start()
    try:
        await _write_ga(xknx, GA_FAN_SPEED, DPTArray(speed))
        payload = await _read_ga(xknx, GA_FAN_SPEED)
        assert isinstance(payload, DPTArray)
        assert payload.value == (speed,)
    finally:
        await xknx.stop()


# --- Push (unsolicited state updates) tests ---


@pytest.mark.asyncio
async def test_receive_pushed_update(knx_tmk_application):
    """Server pushes GroupValueWrite when thermostat state changes."""
    received_values: list[float] = []

    def on_telegram(telegram: Telegram) -> None:
        if str(telegram.destination_address) != str(GA_SETPOINT):
            return
        payload = telegram.payload
        if not isinstance(payload, GroupValueWrite):
            return
        value = payload.value
        if isinstance(value, DPTArray) and len(value.value) == 2:
            received_values.append(DPTTemperature.from_knx(value))

    xknx = XKNX(
        connection_config=_connection_config(),
        telegram_received_cb=on_telegram,
    )
    await xknx.start()
    try:
        # Write a new setpoint — the server should push it back as GroupValueWrite.
        await _write_ga(xknx, GA_SETPOINT, _encode_dpt9(20.5))

        # Wait for the pushed update to arrive (sync_interval=1s by default).
        for _ in range(30):
            if received_values:
                break
            await asyncio.sleep(0.2)

        assert len(received_values) > 0, "No pushed setpoint update received"
        assert abs(received_values[-1] - 20.5) < 0.5
    finally:
        # Restore default setpoint so subsequent tests aren't affected.
        await _write_ga(xknx, GA_SETPOINT, _encode_dpt9(22.0))
        await xknx.stop()
