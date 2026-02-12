package bacnetctrl

import (
	"context"
	"math"
	"net"
	"testing"
	"time"

	"github.com/ulbios/bacnet"
	"github.com/ulbios/bacnet/objects"
	"github.com/ulbios/bacnet/plumbing"
	"github.com/ulbios/bacnet/services"

	"github.com/Agrid-Dev/thermocktat/internal/testutil"
	"github.com/Agrid-Dev/thermocktat/internal/thermostat"
)

// --- helpers ---

func findFreeUDPAddr(t *testing.T) string {
	t.Helper()
	l, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free udp addr: %v", err)
	}
	a := l.LocalAddr().String()
	_ = l.Close()
	return a
}

// startController creates a controller with a FakeThermostatService and returns
// a connected UDP conn and cleanup function. Optional setup functions run on the
// fake BEFORE the controller starts to avoid data races.
func startController(t *testing.T, opts ...func(*testutil.FakeThermostatService)) (*testutil.FakeThermostatService, *net.UDPConn, func()) {
	t.Helper()
	fake := testutil.NewFakeThermostatService()
	for _, opt := range opts {
		opt(fake)
	}
	addr := findFreeUDPAddr(t)

	ctrl, err := New(fake, Config{
		DeviceID:       "test-dev",
		DeviceInstance: 42,
		Addr:           addr,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = ctrl.Run(ctx) }()
	time.Sleep(50 * time.Millisecond)

	serverAddr, _ := net.ResolveUDPAddr("udp4", addr)
	conn, err := net.DialUDP("udp4", nil, serverAddr)
	if err != nil {
		cancel()
		t.Fatalf("dial: %v", err)
	}

	cleanup := func() {
		conn.Close()
		cancel()
	}
	return fake, conn, cleanup
}

// sendAndReceive sends data and reads the response with a timeout.
func sendAndReceive(t *testing.T, conn *net.UDPConn, data []byte) []byte {
	t.Helper()
	if _, err := conn.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return buf[:n]
}

func buildReadProperty(objectType uint16, instance uint32, propertyId uint8) []byte {
	b, err := bacnet.NewReadProperty(objectType, instance, propertyId)
	if err != nil {
		panic(err)
	}
	return b
}

func buildWriteProperty(objectType uint16, instance uint32, propertyId uint8, value float32) []byte {
	b, err := bacnet.NewWriteProperty(objectType, instance, propertyId, value)
	if err != nil {
		panic(err)
	}
	return b
}

func buildWhoIs() []byte {
	bvlc := plumbing.NewBVLC(plumbing.BVLCFuncUnicast)
	npdu := plumbing.NewNPDU(false, false, false, false)
	apdu := plumbing.NewAPDU(plumbing.UnConfirmedReq, services.ServiceUnconfirmedWhoIs, nil)
	whois := &services.UnconfirmedWhoIs{BVLC: bvlc, NPDU: npdu, APDU: apdu}
	whois.SetLength()
	b, err := whois.MarshalBinary()
	if err != nil {
		panic(err)
	}
	return b
}

// parseComplexACKValue parses a ComplexACK response and returns just the PresentValue.
// Uses our own decodeObjectIdentifier to work around the library's bit-shift bug.
func parseComplexACKValue(t *testing.T, data []byte) float32 {
	t.Helper()
	pkt, err := bacnet.Parse(data)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	cack, ok := pkt.(*services.ComplexACK)
	if !ok {
		t.Fatalf("expected ComplexACK, got %T", pkt)
	}
	// ComplexACK objects after parsing (opening/closing tags stripped):
	// [0] ObjectIdentifier, [1] PropertyIdentifier, [2] Real(value)
	if len(cack.APDU.Objects) < 3 {
		t.Fatalf("ComplexACK objects: got %d, want >= 3", len(cack.APDU.Objects))
	}
	val, err := decodeReal(cack.APDU.Objects[2])
	if err != nil {
		t.Fatalf("decode ComplexACK value: %v", err)
	}
	return val
}

// assertSimpleACK asserts the response is a SimpleACK for the given service.
func assertSimpleACK(t *testing.T, data []byte, service uint8) {
	t.Helper()
	pkt, err := bacnet.Parse(data)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	sack, ok := pkt.(*services.SimpleACK)
	if !ok {
		t.Fatalf("expected SimpleACK, got %T", pkt)
	}
	if sack.APDU.Service != service {
		t.Fatalf("SimpleACK service: got %d want %d", sack.APDU.Service, service)
	}
}

// assertErrorResponse asserts the response is an Error with the given class and code.
func assertErrorResponse(t *testing.T, data []byte, errClass, errCode uint8) {
	t.Helper()
	pkt, err := bacnet.Parse(data)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	errMsg, ok := pkt.(*services.Error)
	if !ok {
		t.Fatalf("expected Error, got %T", pkt)
	}
	dec, err := errMsg.Decode()
	if err != nil {
		t.Fatalf("decode Error: %v", err)
	}
	if dec.ErrorClass != errClass || dec.ErrorCode != errCode {
		t.Fatalf("Error class/code: got %d/%d want %d/%d", dec.ErrorClass, dec.ErrorCode, errClass, errCode)
	}
}

func almostEqual(a, b, epsilon float32) bool {
	return float32(math.Abs(float64(a-b))) < epsilon
}

// readValue is a helper that sends a ReadProperty and returns the float value.
func readValue(t *testing.T, conn *net.UDPConn, objType uint16, instance uint32) float32 {
	t.Helper()
	resp := sendAndReceive(t, conn, buildReadProperty(objType, instance, objects.PropertyIdPresentValue))
	return parseComplexACKValue(t, resp)
}

// writeValue is a helper that sends a WriteProperty and asserts SimpleACK.
func writeValue(t *testing.T, conn *net.UDPConn, objType uint16, instance uint32, value float32) {
	t.Helper()
	resp := sendAndReceive(t, conn, buildWriteProperty(objType, instance, objects.PropertyIdPresentValue, value))
	assertSimpleACK(t, resp, services.ServiceConfirmedWriteProperty)
}

// --- Who-Is / I-Am ---

func TestWhoIs_IAm(t *testing.T) {
	_, conn, cleanup := startController(t)
	defer cleanup()

	resp := sendAndReceive(t, conn, buildWhoIs())

	pkt, err := bacnet.Parse(resp)
	if err != nil {
		t.Fatalf("parse I-Am: %v", err)
	}
	iam, ok := pkt.(*services.UnconfirmedIAm)
	if !ok {
		t.Fatalf("expected I-Am, got %T", pkt)
	}
	// Use our own decoder because the library's DecObjectIdentifier has a bit-shift bug.
	if len(iam.APDU.Objects) < 1 {
		t.Fatal("I-Am: no objects")
	}
	_, instN, err := decodeObjectIdentifier(iam.APDU.Objects[0])
	if err != nil {
		t.Fatalf("decode I-Am object: %v", err)
	}
	if instN != 42 {
		t.Fatalf("device instance: got %d want 42", instN)
	}
}

// --- ReadProperty ---
// Initial values are set via startController opts (before the controller goroutine starts)
// to avoid data races between the test goroutine and the controller goroutine.

func TestReadProperty_AmbientTemperature(t *testing.T) {
	_, conn, cleanup := startController(t, func(f *testutil.FakeThermostatService) {
		f.S.AmbientTemperature = 21.5
	})
	defer cleanup()

	val := readValue(t, conn, objects.ObjectTypeAnalogInput, 0)
	if !almostEqual(val, 21.5, 0.01) {
		t.Fatalf("ambient_temperature: got %f want 21.5", val)
	}
}

func TestReadProperty_TemperatureSetpoint(t *testing.T) {
	_, conn, cleanup := startController(t) // default: 22.0
	defer cleanup()

	val := readValue(t, conn, ObjectTypeAnalogValue, 0)
	if !almostEqual(val, 22.0, 0.01) {
		t.Fatalf("setpoint: got %f want 22.0", val)
	}
}

func TestReadProperty_TemperatureSetpointMin(t *testing.T) {
	_, conn, cleanup := startController(t) // default: 16.0
	defer cleanup()

	val := readValue(t, conn, ObjectTypeAnalogValue, 1)
	if !almostEqual(val, 16.0, 0.01) {
		t.Fatalf("setpoint_min: got %f want 16.0", val)
	}
}

func TestReadProperty_TemperatureSetpointMax(t *testing.T) {
	_, conn, cleanup := startController(t) // default: 28.0
	defer cleanup()

	val := readValue(t, conn, ObjectTypeAnalogValue, 2)
	if !almostEqual(val, 28.0, 0.01) {
		t.Fatalf("setpoint_max: got %f want 28.0", val)
	}
}

func TestReadProperty_Enabled(t *testing.T) {
	// Default: enabled=true
	_, conn, cleanup := startController(t)
	defer cleanup()

	val := readValue(t, conn, ObjectTypeBinaryValue, 0)
	if val != 1 {
		t.Fatalf("enabled=true: got %f want 1", val)
	}

	// Write false via BACnet, then read back (avoids race on fake.S)
	writeValue(t, conn, ObjectTypeBinaryValue, 0, 0)
	val = readValue(t, conn, ObjectTypeBinaryValue, 0)
	if val != 0 {
		t.Fatalf("enabled=false: got %f want 0", val)
	}
}

func TestReadProperty_Mode(t *testing.T) {
	_, conn, cleanup := startController(t, func(f *testutil.FakeThermostatService) {
		f.S.Mode = thermostat.ModeCool // 2
	})
	defer cleanup()

	val := readValue(t, conn, ObjectTypeMultiStateValue, 0)
	if val != 2 {
		t.Fatalf("mode: got %f want 2", val)
	}
}

func TestReadProperty_FanSpeed(t *testing.T) {
	_, conn, cleanup := startController(t, func(f *testutil.FakeThermostatService) {
		f.S.FanSpeed = thermostat.FanHigh // 4
	})
	defer cleanup()

	val := readValue(t, conn, ObjectTypeMultiStateValue, 1)
	if val != 4 {
		t.Fatalf("fan_speed: got %f want 4", val)
	}
}

// --- WriteProperty ---
// Writes are verified via ReadProperty read-back rather than checking fake internals,
// since the controller goroutine and test goroutine share the fake without a mutex.

func TestWriteProperty_TemperatureSetpoint(t *testing.T) {
	_, conn, cleanup := startController(t)
	defer cleanup()

	writeValue(t, conn, ObjectTypeAnalogValue, 0, 24.5)
	val := readValue(t, conn, ObjectTypeAnalogValue, 0)
	if !almostEqual(val, 24.5, 0.01) {
		t.Fatalf("setpoint: got %f want 24.5", val)
	}
}

func TestWriteProperty_Enabled(t *testing.T) {
	_, conn, cleanup := startController(t)
	defer cleanup()

	writeValue(t, conn, ObjectTypeBinaryValue, 0, 0)
	val := readValue(t, conn, ObjectTypeBinaryValue, 0)
	if val != 0 {
		t.Fatalf("enabled: got %f want 0", val)
	}
}

func TestWriteProperty_Mode(t *testing.T) {
	_, conn, cleanup := startController(t)
	defer cleanup()

	writeValue(t, conn, ObjectTypeMultiStateValue, 0, 1) // ModeHeat
	val := readValue(t, conn, ObjectTypeMultiStateValue, 0)
	if val != 1 {
		t.Fatalf("mode: got %f want 1 (heat)", val)
	}
}

func TestWriteProperty_FanSpeed(t *testing.T) {
	_, conn, cleanup := startController(t)
	defer cleanup()

	writeValue(t, conn, ObjectTypeMultiStateValue, 1, 3) // FanMedium
	val := readValue(t, conn, ObjectTypeMultiStateValue, 1)
	if val != 3 {
		t.Fatalf("fan_speed: got %f want 3 (medium)", val)
	}
}

func TestWriteProperty_SetpointMin(t *testing.T) {
	_, conn, cleanup := startController(t)
	defer cleanup()

	writeValue(t, conn, ObjectTypeAnalogValue, 1, 18.0)
	val := readValue(t, conn, ObjectTypeAnalogValue, 1)
	if !almostEqual(val, 18.0, 0.01) {
		t.Fatalf("setpoint_min: got %f want 18.0", val)
	}
}

func TestWriteProperty_SetpointMax(t *testing.T) {
	_, conn, cleanup := startController(t)
	defer cleanup()

	writeValue(t, conn, ObjectTypeAnalogValue, 2, 30.0)
	val := readValue(t, conn, ObjectTypeAnalogValue, 2)
	if !almostEqual(val, 30.0, 0.01) {
		t.Fatalf("setpoint_max: got %f want 30.0", val)
	}
}

// --- Error cases ---

func TestReadProperty_UnknownObject(t *testing.T) {
	_, conn, cleanup := startController(t)
	defer cleanup()

	// AnalogInput instance 99 does not exist
	resp := sendAndReceive(t, conn, buildReadProperty(objects.ObjectTypeAnalogInput, 99, objects.PropertyIdPresentValue))
	assertErrorResponse(t, resp, objects.ErrorClassObject, objects.ErrorCodeUnknownObject)
}

func TestReadProperty_UnsupportedProperty(t *testing.T) {
	_, conn, cleanup := startController(t)
	defer cleanup()

	// Property 77 (ObjectName) is not PresentValue — not supported
	resp := sendAndReceive(t, conn, buildReadProperty(ObjectTypeAnalogValue, 0, 77))
	assertErrorResponse(t, resp, objects.ErrorClassService, objects.ErrorCodeServiceRequestDenied)
}

func TestWriteProperty_ReadOnlyObject(t *testing.T) {
	_, conn, cleanup := startController(t)
	defer cleanup()

	// AnalogInput 0 (ambient_temperature) is read-only
	resp := sendAndReceive(t, conn, buildWriteProperty(objects.ObjectTypeAnalogInput, 0, objects.PropertyIdPresentValue, 25.0))
	assertErrorResponse(t, resp, objects.ErrorClassService, objects.ErrorCodeServiceRequestDenied)
}

func TestWriteProperty_UnknownObject(t *testing.T) {
	_, conn, cleanup := startController(t)
	defer cleanup()

	resp := sendAndReceive(t, conn, buildWriteProperty(ObjectTypeAnalogValue, 99, objects.PropertyIdPresentValue, 25.0))
	assertErrorResponse(t, resp, objects.ErrorClassObject, objects.ErrorCodeUnknownObject)
}
