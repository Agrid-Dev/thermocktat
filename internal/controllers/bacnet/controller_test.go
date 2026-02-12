package bacnetctrl

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/ulbios/bacnet"
	"github.com/ulbios/bacnet/plumbing"
	"github.com/ulbios/bacnet/services"

	"github.com/Agrid-Dev/thermocktat/internal/thermostat"
)

// findFreeUDPAddr finds an available UDP address (bind then close) and returns its string.
func findFreeUDPAddr(t *testing.T) string {
	l, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free udp addr: %v", err)
	}
	a := l.LocalAddr().String()
	_ = l.Close()
	return a
}

func TestWhoIs_IAm(t *testing.T) {
	// Minimal stub implementing the ThermostatService interface; WhoIs/I-Am test doesn't use it.
	stub := &testThermostatServiceStub{}

	addr := findFreeUDPAddr(t)

	ctrl, err := New(stub, Config{
		DeviceID:       "dev-1",
		DeviceInstance: 321, // sample instance to verify in response
		Addr:           addr,
	})
	if err != nil {
		t.Fatalf("New controller: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// run controller
	go func() {
		_ = ctrl.Run(ctx)
	}()

	// wait a bit for listener to start
	time.Sleep(50 * time.Millisecond)

	// prepare Who-Is targeted to controller address
	bvlc := plumbing.NewBVLC(plumbing.BVLCFuncUnicast)
	npdu := plumbing.NewNPDU(false, false, false, false)
	apdu := plumbing.NewAPDU(plumbing.UnConfirmedReq, services.ServiceUnconfirmedWhoIs, nil)

	whois := &services.UnconfirmedWhoIs{
		BVLC: bvlc,
		NPDU: npdu,
		APDU: apdu,
	}
	whois.SetLength()
	data, err := whois.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal whois: %v", err)
	}

	// Dial UDP (local bind) so we can receive I-Am reply.
	serverAddr, _ := net.ResolveUDPAddr("udp4", addr)
	conn, err := net.DialUDP("udp4", nil, serverAddr)
	if err != nil {
		t.Fatalf("dial udp: %v", err)
	}
	defer conn.Close()

	// send Who-Is
	if _, err := conn.Write(data); err != nil {
		t.Fatalf("send whois: %v", err)
	}

	// wait for response
	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read reply: %v", err)
	}

	// Parse the incoming packet using top-level bacnet.Parse
	pkt, err := bacnet.Parse(buf[:n])
	if err != nil {
		t.Fatalf("parse reply: %v", err)
	}

	iam, ok := pkt.(*services.UnconfirmedIAm)
	if !ok {
		t.Fatalf("expected UnconfirmedIAm, got %T", pkt)
	}

	dec, err := iam.Decode()
	if err != nil {
		t.Fatalf("decode I-Am: %v", err)
	}

	if dec.DeviceId != uint32(321) {
		t.Fatalf("unexpected device id in I-Am: got %d want %d", dec.DeviceId, 321)
	}

	// done
	cancel()
}

// testThermostatService is a more complete stub for testing ReadProperty/WriteProperty
type testThermostatService struct {
	mu       sync.Mutex
	snapshot thermostat.Snapshot
}

func newTestThermostatService() *testThermostatService {
	return &testThermostatService{
		snapshot: thermostat.Snapshot{
			Enabled:                true,
			TemperatureSetpoint:    22.0,
			TemperatureSetpointMin: 16.0,
			TemperatureSetpointMax: 28.0,
			Mode:                   thermostat.ModeAuto,
			FanSpeed:               thermostat.FanAuto,
			AmbientTemperature:     21.0,
		},
	}
}

func (s *testThermostatService) Get() thermostat.Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot
}

func (s *testThermostatService) SetEnabled(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot.Enabled = v
}

func (s *testThermostatService) SetSetpoint(v float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot.TemperatureSetpoint = v
	return nil
}

func (s *testThermostatService) SetMinMax(min, max float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot.TemperatureSetpointMin = min
	s.snapshot.TemperatureSetpointMax = max
	return nil
}

func (s *testThermostatService) SetMode(m thermostat.Mode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot.Mode = m
	return nil
}

func (s *testThermostatService) SetFanSpeed(f thermostat.FanSpeed) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot.FanSpeed = f
	return nil
}

// Minimal stub implementing ports.ThermostatService for test (WhoIs doesn't use the service).
type testThermostatServiceStub struct{}

func (s *testThermostatServiceStub) Get() thermostat.Snapshot {
	return thermostat.Snapshot{}
}
func (s *testThermostatServiceStub) SetEnabled(bool)                         {}
func (s *testThermostatServiceStub) SetSetpoint(float64) error               { return nil }
func (s *testThermostatServiceStub) SetMinMax(min, max float64) error        { return nil }
func (s *testThermostatServiceStub) SetMode(m thermostat.Mode) error         { return nil }
func (s *testThermostatServiceStub) SetFanSpeed(f thermostat.FanSpeed) error { return nil }
