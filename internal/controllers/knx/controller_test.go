package knxctrl

import (
	"context"
	"math"
	"net"
	"testing"
	"time"

	"github.com/Agrid-Dev/thermocktat/internal/testutil"
	"github.com/Agrid-Dev/thermocktat/internal/thermostat"
)

// startController starts a controller on a random port and returns
// a connected UDP socket and cleanup function.
func startController(t *testing.T, setup func(*testutil.FakeThermostatService)) (*testutil.FakeThermostatService, *net.UDPConn, func()) {
	t.Helper()
	fake := testutil.NewFakeThermostatService()
	if setup != nil {
		setup(fake)
	}

	ctrl, err := New(nil, fake, Config{
		DeviceID:        "test",
		Addr:            "127.0.0.1:0",
		PublishInterval: 1 * time.Hour, // effectively disabled for request/response tests
		GAMain:          1,
		GAMiddle:        0,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = ctrl.Run(ctx)
		close(done)
	}()

	// Wait for server to bind.
	time.Sleep(50 * time.Millisecond)

	addr := ctrl.LocalAddr().(*net.UDPAddr)
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		cancel()
		t.Fatalf("dial: %v", err)
	}
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	cleanup := func() {
		conn.Close()
		cancel()
		<-done
	}

	return fake, conn, cleanup
}

// connect sends a CONNECT_REQUEST and returns the channelID.
func connect(t *testing.T, conn *net.UDPConn) uint8 {
	t.Helper()
	pkt := BuildConnectRequest(net.IPv4zero, 0)
	if _, err := conn.Write(pkt); err != nil {
		t.Fatalf("send connect: %v", err)
	}

	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read connect response: %v", err)
	}

	channelID, status, err := ParseConnectResponse(buf[:n])
	if err != nil {
		t.Fatalf("parse connect response: %v", err)
	}
	if status != statusOK {
		t.Fatalf("connect status: 0x%02X", status)
	}
	return channelID
}

func readGA(t *testing.T, conn *net.UDPConn, channelID uint8, seq *uint8, ga uint16) []byte {
	t.Helper()
	pkt := BuildTunnelingGroupValueRead(channelID, *seq, ga)
	*seq++
	if _, err := conn.Write(pkt); err != nil {
		t.Fatalf("send read: %v", err)
	}

	// Read ACK.
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read ack: %v", err)
	}
	h, _ := ParseHeader(buf[:n])
	if h.ServiceType != ServiceTunnelingACK {
		t.Fatalf("expected ACK, got 0x%04X", h.ServiceType)
	}

	// Read L_DATA_CON (confirmation), then the actual response.
	n, err = conn.Read(buf)
	if err != nil {
		t.Fatalf("read l_data_con: %v", err)
	}
	expectLDataCon(t, buf[:n])

	// Read response (TUNNELING_REQUEST from server with GroupValueResponse).
	n, err = conn.Read(buf)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return buf[:n]
}

func writeGA(t *testing.T, conn *net.UDPConn, channelID uint8, seq *uint8, ga uint16, data []byte, compact bool) {
	t.Helper()
	pkt := BuildTunnelingGroupValueWrite(channelID, *seq, ga, data, compact)
	*seq++
	if _, err := conn.Write(pkt); err != nil {
		t.Fatalf("send write: %v", err)
	}

	// Read ACK.
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read ack: %v", err)
	}
	h, _ := ParseHeader(buf[:n])
	if h.ServiceType != ServiceTunnelingACK {
		t.Fatalf("expected ACK, got 0x%04X", h.ServiceType)
	}

	// Read L_DATA_CON (confirmation).
	n, err = conn.Read(buf)
	if err != nil {
		t.Fatalf("read l_data_con: %v", err)
	}
	expectLDataCon(t, buf[:n])
}

// expectLDataCon verifies a packet is a TUNNELING_REQUEST containing an L_Data.con CEMI.
func expectLDataCon(t *testing.T, pkt []byte) {
	t.Helper()
	h, err := ParseHeader(pkt)
	if err != nil {
		t.Fatalf("l_data_con: bad header: %v", err)
	}
	if h.ServiceType != ServiceTunnelingRequest {
		t.Fatalf("l_data_con: expected TUNNELING_REQUEST, got 0x%04X", h.ServiceType)
	}
	cemiData := pkt[headerSize+4:] // skip header + 4-byte tunneling header
	if len(cemiData) < 1 {
		t.Fatal("l_data_con: empty CEMI")
	}
	if cemiData[0] != CEMIMsgCodeLDataCon {
		t.Fatalf("l_data_con: expected msg code 0x2E, got 0x%02X", cemiData[0])
	}
}

// --- Tests ---

func TestConnect(t *testing.T) {
	_, conn, cleanup := startController(t, nil)
	defer cleanup()

	ch := connect(t, conn)
	if ch != 1 {
		t.Fatalf("channelID: got %d, want 1", ch)
	}
}

func TestConnectRejectsSecondClient(t *testing.T) {
	_, conn, cleanup := startController(t, nil)
	defer cleanup()

	connect(t, conn)

	// Second connect from same socket should be rejected.
	pkt := BuildConnectRequest(net.IPv4zero, 0)
	if _, err := conn.Write(pkt); err != nil {
		t.Fatalf("send: %v", err)
	}

	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	_, status, _ := ParseConnectResponse(buf[:n])
	if status != statusNoMore {
		t.Fatalf("expected NO_MORE_CONNECTIONS (0x22), got 0x%02X", status)
	}
}

func TestDisconnect(t *testing.T) {
	_, conn, cleanup := startController(t, nil)
	defer cleanup()

	ch := connect(t, conn)

	pkt := BuildDisconnectRequest(ch, net.IPv4zero, 0)
	if _, err := conn.Write(pkt); err != nil {
		t.Fatalf("send: %v", err)
	}

	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	h, _ := ParseHeader(buf[:n])
	if h.ServiceType != ServiceDisconnectResponse {
		t.Fatalf("expected disconnect response, got 0x%04X", h.ServiceType)
	}
}

func TestConnectionState(t *testing.T) {
	_, conn, cleanup := startController(t, nil)
	defer cleanup()

	ch := connect(t, conn)

	pkt := BuildConnectionStateRequest(ch, net.IPv4zero, 0)
	if _, err := conn.Write(pkt); err != nil {
		t.Fatalf("send: %v", err)
	}

	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	h, _ := ParseHeader(buf[:n])
	if h.ServiceType != ServiceConnectionStateResponse {
		t.Fatalf("expected conn state response, got 0x%04X", h.ServiceType)
	}
	if buf[headerSize+1] != statusOK {
		t.Fatalf("status: 0x%02X", buf[headerSize+1])
	}
}

func TestReadSetpoint(t *testing.T) {
	_, conn, cleanup := startController(t, func(f *testutil.FakeThermostatService) {
		f.S.TemperatureSetpoint = 21.5
	})
	defer cleanup()

	ch := connect(t, conn)
	var seq uint8

	ga := GroupAddress(1, 0, SubSetpoint)
	resp := readGA(t, conn, ch, &seq, ga)
	val, err := DecodeResponseFloat(resp)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if math.Abs(val-21.5) > 0.5 {
		t.Fatalf("setpoint: got %f, want ~21.5", val)
	}
}

func TestReadEnabled(t *testing.T) {
	_, conn, cleanup := startController(t, func(f *testutil.FakeThermostatService) {
		f.S.Enabled = true
	})
	defer cleanup()

	ch := connect(t, conn)
	var seq uint8

	ga := GroupAddress(1, 0, SubEnabled)
	resp := readGA(t, conn, ch, &seq, ga)
	val, err := DecodeResponseBool(resp)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !val {
		t.Fatal("expected enabled=true")
	}
}

func TestReadMode(t *testing.T) {
	_, conn, cleanup := startController(t, func(f *testutil.FakeThermostatService) {
		f.S.Mode = thermostat.ModeCool
	})
	defer cleanup()

	ch := connect(t, conn)
	var seq uint8

	ga := GroupAddress(1, 0, SubMode)
	resp := readGA(t, conn, ch, &seq, ga)
	val, err := DecodeResponseByte(resp)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if thermostat.Mode(val) != thermostat.ModeCool {
		t.Fatalf("mode: got %d, want %d", val, thermostat.ModeCool)
	}
}

func TestReadFanSpeed(t *testing.T) {
	_, conn, cleanup := startController(t, func(f *testutil.FakeThermostatService) {
		f.S.FanSpeed = thermostat.FanHigh
	})
	defer cleanup()

	ch := connect(t, conn)
	var seq uint8

	ga := GroupAddress(1, 0, SubFanSpeed)
	resp := readGA(t, conn, ch, &seq, ga)
	val, err := DecodeResponseByte(resp)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if thermostat.FanSpeed(val) != thermostat.FanHigh {
		t.Fatalf("fan: got %d, want %d", val, thermostat.FanHigh)
	}
}

func TestWriteSetpoint(t *testing.T) {
	_, conn, cleanup := startController(t, nil)
	defer cleanup()

	ch := connect(t, conn)
	var seq uint8

	ga := GroupAddress(1, 0, SubSetpoint)
	encoded := EncodeDPT9(20.0)
	writeGA(t, conn, ch, &seq, ga, encoded[:], false)

	// Verify via read-back.
	resp := readGA(t, conn, ch, &seq, ga)
	val, err := DecodeResponseFloat(resp)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if math.Abs(val-20.0) > 0.5 {
		t.Fatalf("setpoint: got %f, want ~20.0", val)
	}
}

func TestWriteEnabled(t *testing.T) {
	_, conn, cleanup := startController(t, nil)
	defer cleanup()

	ch := connect(t, conn)
	var seq uint8

	ga := GroupAddress(1, 0, SubEnabled)
	writeGA(t, conn, ch, &seq, ga, []byte{0}, true) // compact: disabled

	// Verify via read-back.
	resp := readGA(t, conn, ch, &seq, ga)
	val, err := DecodeResponseBool(resp)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if val != false {
		t.Fatal("expected enabled=false")
	}
}

func TestWriteMode(t *testing.T) {
	_, conn, cleanup := startController(t, nil)
	defer cleanup()

	ch := connect(t, conn)
	var seq uint8

	ga := GroupAddress(1, 0, SubMode)
	writeGA(t, conn, ch, &seq, ga, []byte{byte(thermostat.ModeHeat)}, false)

	// Verify via read-back.
	resp := readGA(t, conn, ch, &seq, ga)
	val, err := DecodeResponseByte(resp)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if thermostat.Mode(val) != thermostat.ModeHeat {
		t.Fatalf("mode: got %v, want heat", thermostat.Mode(val))
	}
}

func TestWriteFanSpeed(t *testing.T) {
	_, conn, cleanup := startController(t, nil)
	defer cleanup()

	ch := connect(t, conn)
	var seq uint8

	ga := GroupAddress(1, 0, SubFanSpeed)
	writeGA(t, conn, ch, &seq, ga, []byte{byte(thermostat.FanLow)}, false)

	// Verify via read-back.
	resp := readGA(t, conn, ch, &seq, ga)
	val, err := DecodeResponseByte(resp)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if thermostat.FanSpeed(val) != thermostat.FanLow {
		t.Fatalf("fan: got %v, want low", thermostat.FanSpeed(val))
	}
}

func TestReadFaultCode(t *testing.T) {
	_, conn, cleanup := startController(t, func(f *testutil.FakeThermostatService) {
		f.S.FaultCode = 513 // spans both bytes (0x0201)
	})
	defer cleanup()

	ch := connect(t, conn)
	var seq uint8

	ga := GroupAddress(1, 0, SubFaultCode)
	resp := readGA(t, conn, ch, &seq, ga)
	raw, _, err := ExtractGroupValueResponseData(resp)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(raw) != 2 {
		t.Fatalf("expected 2 bytes, got %d", len(raw))
	}
	got := int(uint16(raw[0])<<8 | uint16(raw[1]))
	if got != 513 {
		t.Fatalf("fault_code: got %d, want 513", got)
	}
}

func TestWriteFaultCode(t *testing.T) {
	_, conn, cleanup := startController(t, nil)
	defer cleanup()

	ch := connect(t, conn)
	var seq uint8

	ga := GroupAddress(1, 0, SubFaultCode)
	writeGA(t, conn, ch, &seq, ga, []byte{0x00, 0x2A}, false)

	// Verify via read-back.
	resp := readGA(t, conn, ch, &seq, ga)
	raw, _, err := ExtractGroupValueResponseData(resp)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := int(uint16(raw[0])<<8 | uint16(raw[1]))
	if got != 42 {
		t.Fatalf("fault_code: got %d, want 42", got)
	}
}

func TestWriteReadBack(t *testing.T) {
	_, conn, cleanup := startController(t, nil)
	defer cleanup()

	ch := connect(t, conn)
	var seq uint8

	ga := GroupAddress(1, 0, SubSetpoint)

	// Write a setpoint.
	encoded := EncodeDPT9(19.0)
	writeGA(t, conn, ch, &seq, ga, encoded[:], false)

	// Read it back.
	resp := readGA(t, conn, ch, &seq, ga)
	val, err := DecodeResponseFloat(resp)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if math.Abs(val-19.0) > 0.5 {
		t.Fatalf("read-back: got %f, want ~19.0", val)
	}
}

func TestStatePush(t *testing.T) {
	// Use the real thermostat (mutex-protected) to avoid races with stateLoop.
	svc, err := thermostat.New(
		nil,
		thermostat.Snapshot{
			Enabled:                true,
			TemperatureSetpoint:    22,
			TemperatureSetpointMin: 16,
			TemperatureSetpointMax: 28,
			Mode:                   thermostat.ModeAuto,
			FanSpeed:               thermostat.FanAuto,
			AmbientTemperature:     21,
		},
		thermostat.PIDRegulatorParams{Kp: 0.001, Ki: 0.001, Kd: 0.01, TargetHysteresis: 1, ModeChangeHysteresis: 2},
		thermostat.HeatLossSimulatorParams{Coefficient: 0, OutdoorTemperature: 10},
	)
	if err != nil {
		t.Fatalf("new thermostat: %v", err)
	}

	ctrl, err := New(nil, svc, Config{
		DeviceID:        "test",
		Addr:            "127.0.0.1:0",
		PublishInterval: 50 * time.Millisecond,
		GAMain:          1,
		GAMiddle:        0,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = ctrl.Run(ctx)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)

	addr := ctrl.LocalAddr().(*net.UDPAddr)
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		cancel()
		t.Fatalf("dial: %v", err)
	}
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	defer func() {
		conn.Close()
		cancel()
		<-done
	}()

	ch := connect(t, conn)
	_ = ch

	// Change setpoint via the thread-safe API — this triggers a state push.
	if err := svc.SetSetpoint(20.0); err != nil {
		t.Fatalf("set setpoint: %v", err)
	}

	// Wait for the pushed GroupValueWrite for setpoint GA.
	buf := make([]byte, 256)
	gaSetpoint := GroupAddress(1, 0, SubSetpoint)
	var received bool
	for range 40 {
		n, err := conn.Read(buf)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		hdr, _ := ParseHeader(buf[:n])
		if hdr.ServiceType != ServiceTunnelingRequest {
			continue
		}

		_, _, cemiData, err := ParseTunnelingResponse(buf[:n])
		if err != nil {
			continue
		}
		cemi, err := ParseCEMI(cemiData)
		if err != nil {
			continue
		}

		if cemi.DstAddr == gaSetpoint && cemi.APCI == APCIGroupValueWrite {
			val := DecodeDPT9([2]byte{cemi.Data[0], cemi.Data[1]})
			if math.Abs(val-20.0) < 1.0 {
				received = true
				break
			}
		}
	}
	if !received {
		t.Fatal("did not receive pushed setpoint update")
	}
}
