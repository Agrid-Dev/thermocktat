import time

import pytest
from pymodbus.client import ModbusTcpClient

# Mode enum values: ModeHeat=1, ModeCool=2, ModeFan=3, ModeAuto=4
modes = {1, 2, 3, 4}

# Holding register addresses
HR_TEMPERATURE_SETPOINT = 0
HR_TEMPERATURE_SETPOINT_MIN = 1
HR_TEMPERATURE_SETPOINT_MAX = 2
HR_MODE = 3
HR_FAN_SPEED = 4

# Input register addresses
IR_AMBIENT_TEMPERATURE = 0

# Coil address
COIL_ENABLED = 0


@pytest.fixture
def modbus_client():
    """Fixture to provide a Modbus TCP client."""
    client = ModbusTcpClient("localhost", port=1502)
    client.connect()
    yield client
    client.close()


def test_read_registers(tmk_application, modbus_client):
    """Read all Modbus registers and verify snapshot structure."""
    hr = modbus_client.read_holding_registers(
        HR_TEMPERATURE_SETPOINT, count=5, device_id=4
    )
    ir = modbus_client.read_input_registers(
        IR_AMBIENT_TEMPERATURE, count=1, device_id=4
    )
    coil = modbus_client.read_coils(COIL_ENABLED, count=1, device_id=4)

    assert not hr.isError()
    assert not ir.isError()
    assert not coil.isError()

    assert len(hr.registers) == 5
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
def test_set_enabled(tmk_application, modbus_client, value):
    modbus_client.write_coil(COIL_ENABLED, value, device_id=4)
    time.sleep(0.2)
    result = modbus_client.read_coils(COIL_ENABLED, count=1, device_id=4)
    assert not result.isError()
    enabled = result.bits[0]
    assert enabled is value


def test_write_temperature_setpoint(tmk_application, modbus_client):
    setpoint = 20.0
    setpoint_encoded = int(setpoint * 100)  # 2000
    modbus_client.write_register(HR_TEMPERATURE_SETPOINT, setpoint_encoded, device_id=4)
    time.sleep(0.2)
    result = modbus_client.read_holding_registers(
        HR_TEMPERATURE_SETPOINT, count=1, device_id=4
    )
    assert not result.isError()
    temperature_setpoint = result.registers[0]
    assert temperature_setpoint == setpoint_encoded


@pytest.mark.parametrize("mode", modes)
def test_write_mode(tmk_application, modbus_client, mode):
    modbus_client.write_register(HR_MODE, mode, device_id=4)
    time.sleep(0.2)
    result = modbus_client.read_holding_registers(HR_MODE, count=1, device_id=4)
    assert not result.isError()
    read_mode = result.registers[0]
    assert read_mode == mode
