"""Integration tests for the BACnet controller.

Uses raw UDP with manual BACnet/IP packet construction to avoid
heavy Python BACnet library dependencies.
"""

import socket
import struct

import pytest

BACNET_ADDR = ("127.0.0.1", 47808)
TIMEOUT = 2.0

# BACnet object types
OBJ_ANALOG_INPUT = 0
OBJ_ANALOG_VALUE = 2
OBJ_BINARY_VALUE = 5
OBJ_MULTI_STATE_VALUE = 19
OBJ_DEVICE = 8

# Property ID
PROP_PRESENT_VALUE = 85

# Thermostat defaults (from config_defaults.yaml)
DEFAULT_SETPOINT = 22.0
DEFAULT_SETPOINT_MIN = 16.0
DEFAULT_SETPOINT_MAX = 28.0
DEFAULT_MODE_AUTO = 4
DEFAULT_FAN_AUTO = 1
DEFAULT_AMBIENT = 21.0


# --- Packet builders ---


def _bvlc(payload: bytes, func_id: int = 0x0A) -> bytes:
    """BVLC header: type=0x81, function, length (includes header)."""
    length = 4 + len(payload)
    return struct.pack("!BBH", 0x81, func_id, length) + payload


def _npdu(expecting_reply: bool = False) -> bytes:
    """Minimal NPDU: version=1, control flags."""
    control = 0x04 if expecting_reply else 0x00
    return struct.pack("!BB", 0x01, control)


def _enc_object_id(context_tag: int, obj_type: int, instance: int) -> bytes:
    """Encode a BACnet Object Identifier (context-tagged, 4 bytes)."""
    tag_byte = (context_tag << 4) | 0x08 | 4  # tagN | class=1 | length=4
    value = (obj_type << 22) | (instance & 0x3FFFFF)
    return struct.pack("!BI", tag_byte, value)


def _enc_property_id(context_tag: int, prop_id: int) -> bytes:
    """Encode a BACnet Property Identifier (context-tagged, 1 byte)."""
    tag_byte = (context_tag << 4) | 0x08 | 1  # tagN | class=1 | length=1
    return struct.pack("!BB", tag_byte, prop_id)


def _enc_real(value: float) -> bytes:
    """Encode an application-tagged Real (float32)."""
    tag_byte = (4 << 4) | 4  # TagReal=4, class=0, length=4
    return struct.pack("!Bf", tag_byte, value)


def _enc_opening_tag(tag_n: int) -> bytes:
    return bytes([0x3E | (tag_n << 4) if tag_n == 3 else (tag_n << 4) | 0x0E])


def _enc_closing_tag(tag_n: int) -> bytes:
    return bytes([0x3F | (tag_n << 4) if tag_n == 3 else (tag_n << 4) | 0x0F])


def _enc_null() -> bytes:
    """Encode application-tagged Null."""
    return bytes([0x00])


def _enc_priority(context_tag: int, priority: int) -> bytes:
    """Encode a context-tagged priority (1 byte)."""
    tag_byte = (context_tag << 4) | 0x08 | 1
    return struct.pack("!BB", tag_byte, priority)


def build_whois() -> bytes:
    """Build a BACnet Who-Is broadcast packet."""
    apdu = struct.pack("!BB", 0x10, 0x08)  # UnconfirmedReq, ServiceWhoIs
    return _bvlc(_npdu() + apdu)


def build_read_property(
    obj_type: int, instance: int, prop_id: int, invoke_id: int = 1
) -> bytes:
    """Build a BACnet ReadProperty request."""
    # APDU: ConfirmedReq header
    apdu_header = struct.pack("!BBBB", 0x00, 0x05, invoke_id, 0x0C)
    objects = _enc_object_id(0, obj_type, instance) + _enc_property_id(1, prop_id)
    payload = _npdu(expecting_reply=True) + apdu_header + objects
    return _bvlc(payload)


def build_write_property(
    obj_type: int,
    instance: int,
    prop_id: int,
    value: float,
    invoke_id: int = 1,
) -> bytes:
    """Build a BACnet WriteProperty request."""
    apdu_header = struct.pack("!BBBB", 0x00, 0x05, invoke_id, 0x0F)
    objects = (
        _enc_object_id(0, obj_type, instance)
        + _enc_property_id(1, prop_id)
        + _enc_opening_tag(3)
        + _enc_real(value)
        + _enc_null()
        + _enc_closing_tag(3)
        + _enc_priority(4, 16)
    )
    payload = _npdu(expecting_reply=True) + apdu_header + objects
    return _bvlc(payload)


# --- Response parsers ---


def _parse_apdu_type(data: bytes) -> int:
    """Return APDU type from raw response (after BVLC+NPDU at offset 6)."""
    return data[6] >> 4


def parse_complex_ack_value(data: bytes) -> float:
    """Extract the float32 PresentValue from a ComplexACK response.

    ComplexACK layout after BVLC(4)+NPDU(2):
      APDU: type(1) invokeID(1) service(1)
      Objects: ObjID(5) PropID(2) OpeningTag(1) Real(5) ClosingTag(1)
    The Real tag+data starts at offset 6+3+5+2+1 = 17.
    """
    # Find the Real value: scan for tag byte 0x44 (TagReal=4, class=0, len=4)
    offset = 6 + 3  # after BVLC+NPDU+APDU header
    end = len(data)
    while offset < end:
        tag_byte = data[offset]
        # Check for opening/closing tags (skip them)
        if tag_byte in (0x3E, 0x3F):
            offset += 1
            continue
        tag_number = tag_byte >> 4
        length = tag_byte & 0x07
        if tag_number == 4 and length == 4:  # Real
            return struct.unpack("!f", data[offset + 1 : offset + 5])[0]
        offset += 1 + length
    raise ValueError("Real value not found in ComplexACK")


def is_simple_ack(data: bytes) -> bool:
    """Check if response is a SimpleACK."""
    return _parse_apdu_type(data) == 2


def is_error(data: bytes) -> bool:
    """Check if response is a BACnet Error."""
    return _parse_apdu_type(data) == 5


def parse_iam_device_instance(data: bytes) -> int:
    """Extract device instance from I-Am response.

    I-Am layout after BVLC(4)+NPDU(2)+APDU(2):
      ObjectIdentifier: tag(1) + data(4)
    The ObjID starts at offset 8.
    """
    oid_data = data[9:13]  # 4 bytes after the tag byte at offset 8
    joined = struct.unpack("!I", oid_data)[0]
    return joined & 0x3FFFFF


# --- Fixtures ---


@pytest.fixture
def bacnet_socket():
    """Provide a UDP socket connected to the BACnet controller."""
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.settimeout(TIMEOUT)
    sock.connect(BACNET_ADDR)
    yield sock
    sock.close()


def send_recv(sock: socket.socket, data: bytes) -> bytes:
    sock.send(data)
    return sock.recv(2048)


# --- Tests ---


def test_whois_iam(bacnet_tmk_application, bacnet_socket):
    """Who-Is should be answered with I-Am containing device instance 1 (default)."""
    resp = send_recv(bacnet_socket, build_whois())
    assert _parse_apdu_type(resp) == 1  # UnconfirmedReq (I-Am is type 0x10 >> 4 = 1)
    instance = parse_iam_device_instance(resp)
    assert instance == 1  # default device_instance from config_defaults.yaml


def test_read_ambient_temperature(bacnet_tmk_application, bacnet_socket):
    pkt = build_read_property(OBJ_ANALOG_INPUT, 0, PROP_PRESENT_VALUE)
    resp = send_recv(bacnet_socket, pkt)
    assert not is_error(resp), "expected ComplexACK, got Error"
    val = parse_complex_ack_value(resp)
    # Ambient may drift from default due to PID regulator + heat loss; just verify it's a sane temperature.
    assert 5.0 < val < 40.0, f"ambient_temperature out of range: {val}"


def test_read_temperature_setpoint(bacnet_tmk_application, bacnet_socket):
    pkt = build_read_property(OBJ_ANALOG_VALUE, 0, PROP_PRESENT_VALUE)
    resp = send_recv(bacnet_socket, pkt)
    assert not is_error(resp), "expected ComplexACK, got Error"
    val = parse_complex_ack_value(resp)
    assert abs(val - DEFAULT_SETPOINT) < 0.1


def test_read_enabled(bacnet_tmk_application, bacnet_socket):
    pkt = build_read_property(OBJ_BINARY_VALUE, 0, PROP_PRESENT_VALUE)
    resp = send_recv(bacnet_socket, pkt)
    assert not is_error(resp)
    val = parse_complex_ack_value(resp)
    assert val == 1.0  # default enabled=true


def test_read_mode(bacnet_tmk_application, bacnet_socket):
    pkt = build_read_property(OBJ_MULTI_STATE_VALUE, 0, PROP_PRESENT_VALUE)
    resp = send_recv(bacnet_socket, pkt)
    assert not is_error(resp)
    val = parse_complex_ack_value(resp)
    assert val == DEFAULT_MODE_AUTO


def test_read_fan_speed(bacnet_tmk_application, bacnet_socket):
    pkt = build_read_property(OBJ_MULTI_STATE_VALUE, 1, PROP_PRESENT_VALUE)
    resp = send_recv(bacnet_socket, pkt)
    assert not is_error(resp)
    val = parse_complex_ack_value(resp)
    assert val == DEFAULT_FAN_AUTO


def test_write_temperature_setpoint(bacnet_tmk_application, bacnet_socket):
    new_setpoint = 25.0
    pkt = build_write_property(OBJ_ANALOG_VALUE, 0, PROP_PRESENT_VALUE, new_setpoint)
    resp = send_recv(bacnet_socket, pkt)
    assert is_simple_ack(resp)

    # Read back
    pkt = build_read_property(OBJ_ANALOG_VALUE, 0, PROP_PRESENT_VALUE)
    resp = send_recv(bacnet_socket, pkt)
    assert not is_error(resp)
    val = parse_complex_ack_value(resp)
    assert abs(val - new_setpoint) < 0.1


def test_write_enabled(bacnet_tmk_application, bacnet_socket):
    # Disable
    pkt = build_write_property(OBJ_BINARY_VALUE, 0, PROP_PRESENT_VALUE, 0.0)
    resp = send_recv(bacnet_socket, pkt)
    assert is_simple_ack(resp)

    pkt = build_read_property(OBJ_BINARY_VALUE, 0, PROP_PRESENT_VALUE)
    resp = send_recv(bacnet_socket, pkt)
    assert not is_error(resp)
    val = parse_complex_ack_value(resp)
    assert val == 0.0

    # Re-enable
    pkt = build_write_property(OBJ_BINARY_VALUE, 0, PROP_PRESENT_VALUE, 1.0)
    resp = send_recv(bacnet_socket, pkt)
    assert is_simple_ack(resp)


@pytest.mark.parametrize("mode", [1, 2, 3, 4])  # heat, cool, fan, auto
def test_write_mode(bacnet_tmk_application, bacnet_socket, mode):
    pkt = build_write_property(
        OBJ_MULTI_STATE_VALUE, 0, PROP_PRESENT_VALUE, float(mode)
    )
    resp = send_recv(bacnet_socket, pkt)
    assert is_simple_ack(resp)

    pkt = build_read_property(OBJ_MULTI_STATE_VALUE, 0, PROP_PRESENT_VALUE)
    resp = send_recv(bacnet_socket, pkt)
    assert not is_error(resp)
    val = parse_complex_ack_value(resp)
    assert val == float(mode)


def test_read_unknown_object_returns_error(bacnet_tmk_application, bacnet_socket):
    pkt = build_read_property(OBJ_ANALOG_INPUT, 99, PROP_PRESENT_VALUE)
    resp = send_recv(bacnet_socket, pkt)
    assert is_error(resp)


def test_write_readonly_returns_error(bacnet_tmk_application, bacnet_socket):
    """Writing to AnalogInput (ambient_temperature) should return Error."""
    pkt = build_write_property(OBJ_ANALOG_INPUT, 0, PROP_PRESENT_VALUE, 25.0)
    resp = send_recv(bacnet_socket, pkt)
    assert is_error(resp)
