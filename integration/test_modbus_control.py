import struct
import time

import pytest
from pymodbus.client import ModbusTcpClient

# Mode enum values: ModeHeat=1, ModeCool=2, ModeFan=3, ModeAuto=4
modes = {1, 2, 3, 4}

# Holding register addresses (spaced by 2 for 32-bit compatibility)
HR_TEMPERATURE_SETPOINT = 0
HR_TEMPERATURE_SETPOINT_MIN = 2
HR_TEMPERATURE_SETPOINT_MAX = 4
HR_MODE = 6
HR_FAN_SPEED = 8
HR_TOTAL = 10

# Input register addresses
IR_AMBIENT_TEMPERATURE = 0
IR_TOTAL = 2

# Coil address
COIL_ENABLED = 0

DEVICE_ID = 4


@pytest.fixture
def modbus_client():
    """Fixture to provide a Modbus TCP client."""
    client = ModbusTcpClient("localhost", port=1502)
    client.connect()
    yield client
    client.close()


@pytest.fixture
def modbus_client_32bit():
    """Fixture to provide a Modbus TCP client for the 32-bit mode instance."""
    client = ModbusTcpClient("localhost", port=1503)
    client.connect()
    yield client
    client.close()


def test_read_registers(modbus_tmk_application, modbus_client):
    """Read all Modbus registers and verify snapshot structure."""
    hr = modbus_client.read_holding_registers(
        HR_TEMPERATURE_SETPOINT, count=HR_TOTAL, device_id=DEVICE_ID
    )
    ir = modbus_client.read_input_registers(
        IR_AMBIENT_TEMPERATURE, count=1, device_id=DEVICE_ID
    )
    coil = modbus_client.read_coils(COIL_ENABLED, count=1, device_id=DEVICE_ID)

    assert not hr.isError()
    assert not ir.isError()
    assert not coil.isError()

    assert len(hr.registers) == HR_TOTAL
    temperature_setpoint = hr.registers[HR_TEMPERATURE_SETPOINT]
    temperature_setpoint_min = hr.registers[HR_TEMPERATURE_SETPOINT_MIN]
    temperature_setpoint_max = hr.registers[HR_TEMPERATURE_SETPOINT_MAX]
    mode = hr.registers[HR_MODE]
    fan_speed = hr.registers[HR_FAN_SPEED]

    assert isinstance(temperature_setpoint, int)  # scaled by 100
    assert isinstance(temperature_setpoint_min, int)
    assert isinstance(temperature_setpoint_max, int)
    assert mode in modes
    assert isinstance(fan_speed, int)

    assert len(ir.registers) == 1
    ambient_temperature = ir.registers[IR_AMBIENT_TEMPERATURE]
    assert isinstance(ambient_temperature, int)  # scaled by 100

    assert len(coil.bits) >= 1
    enabled = coil.bits[COIL_ENABLED]
    assert isinstance(enabled, bool)


@pytest.mark.parametrize("value", [True, False])
def test_set_enabled(modbus_tmk_application, modbus_client, value):
    modbus_client.write_coil(COIL_ENABLED, value, device_id=DEVICE_ID)
    time.sleep(0.2)
    result = modbus_client.read_coils(COIL_ENABLED, count=1, device_id=DEVICE_ID)
    assert not result.isError()
    enabled = result.bits[0]
    assert enabled is value


def test_write_temperature_setpoint(modbus_tmk_application, modbus_client):
    setpoint = 20.0
    setpoint_encoded = int(setpoint * 100)  # 2000
    modbus_client.write_register(
        HR_TEMPERATURE_SETPOINT, setpoint_encoded, device_id=DEVICE_ID
    )
    time.sleep(0.2)
    result = modbus_client.read_holding_registers(
        HR_TEMPERATURE_SETPOINT, count=1, device_id=DEVICE_ID
    )
    assert not result.isError()
    temperature_setpoint = result.registers[0]
    assert temperature_setpoint == setpoint_encoded


@pytest.mark.parametrize("mode", modes)
def test_write_mode(modbus_tmk_application, modbus_client, mode):
    modbus_client.write_register(HR_MODE, mode, device_id=DEVICE_ID)
    time.sleep(0.2)
    result = modbus_client.read_holding_registers(HR_MODE, count=1, device_id=DEVICE_ID)
    assert not result.isError()
    read_mode = result.registers[0]
    assert read_mode == mode


# --- 32-bit mode tests ---


def _encode_float32(value):
    """Encode a float as two big-endian uint16 registers (IEEE 754 float32)."""
    packed = struct.pack(">f", value)
    hi, lo = struct.unpack(">HH", packed)
    return [hi, lo]


def _decode_float32(hi, lo):
    """Decode two big-endian uint16 registers into a float (IEEE 754 float32)."""
    packed = struct.pack(">HH", hi, lo)
    return struct.unpack(">f", packed)[0]


def test_read_registers_32bit(modbus_tmk_application_32bit, modbus_client_32bit):
    """Read holding registers in 32-bit mode and verify float32 encoding."""
    hr = modbus_client_32bit.read_holding_registers(
        HR_TEMPERATURE_SETPOINT, count=HR_TOTAL, device_id=DEVICE_ID
    )
    assert not hr.isError()
    assert len(hr.registers) == HR_TOTAL

    # Temperature setpoint should be float32 across 2 registers
    sp = _decode_float32(
        hr.registers[HR_TEMPERATURE_SETPOINT],
        hr.registers[HR_TEMPERATURE_SETPOINT + 1],
    )
    assert abs(sp - 22.0) < 0.01

    # Mode is still a plain uint16
    mode = hr.registers[HR_MODE]
    assert mode in modes

    # Read input registers (ambient temp as float32)
    ir = modbus_client_32bit.read_input_registers(
        IR_AMBIENT_TEMPERATURE, count=IR_TOTAL, device_id=DEVICE_ID
    )
    assert not ir.isError()
    assert len(ir.registers) == IR_TOTAL
    ambient = _decode_float32(
        ir.registers[IR_AMBIENT_TEMPERATURE],
        ir.registers[IR_AMBIENT_TEMPERATURE + 1],
    )
    assert abs(ambient - 21.0) < 0.01


def test_write_temperature_setpoint_32bit(
    modbus_tmk_application_32bit, modbus_client_32bit
):
    """Write a float32 temperature setpoint using write_registers (function 16)."""
    setpoint = 25.75
    regs = _encode_float32(setpoint)
    result = modbus_client_32bit.write_registers(
        HR_TEMPERATURE_SETPOINT, regs, device_id=DEVICE_ID
    )
    assert not result.isError()

    time.sleep(0.2)

    hr = modbus_client_32bit.read_holding_registers(
        HR_TEMPERATURE_SETPOINT, count=2, device_id=DEVICE_ID
    )
    assert not hr.isError()
    read_sp = _decode_float32(hr.registers[0], hr.registers[1])
    assert abs(read_sp - setpoint) < 0.01


def test_write_mode_32bit(modbus_tmk_application_32bit, modbus_client_32bit):
    """Write mode enum (single register) in 32-bit mode via function 6."""
    mode = 1  # ModeHeat
    result = modbus_client_32bit.write_register(HR_MODE, mode, device_id=DEVICE_ID)
    assert not result.isError()

    time.sleep(0.2)

    hr = modbus_client_32bit.read_holding_registers(
        HR_MODE, count=1, device_id=DEVICE_ID
    )
    assert not hr.isError()
    assert hr.registers[0] == mode
