package modbusctrl

import (
	"encoding/binary"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/goburrow/modbus"

	"github.com/Agrid-Dev/thermocktat/internal/thermostat"
)

// fake service for tests
type spyThermostatService struct {
	mu sync.Mutex
	s  thermostat.Snapshot

	// record calls
	setEnabledCalls  []bool
	setSetpointCalls []float64
	setMinMaxCalls   [][2]float64
	setModeCalls     []thermostat.Mode
	setFanCalls      []thermostat.FanSpeed
}

func (f *spyThermostatService) Get() thermostat.Snapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.s
}
func (f *spyThermostatService) SetEnabled(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.s.Enabled = v
	f.setEnabledCalls = append(f.setEnabledCalls, v)
}
func (f *spyThermostatService) SetSetpoint(v float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.s.TemperatureSetpoint = v
	f.setSetpointCalls = append(f.setSetpointCalls, v)
	return nil
}
func (f *spyThermostatService) SetMinMax(min, max float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.s.TemperatureSetpointMin = min
	f.s.TemperatureSetpointMax = max
	f.setMinMaxCalls = append(f.setMinMaxCalls, [2]float64{min, max})
	return nil
}
func (f *spyThermostatService) SetMode(m thermostat.Mode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.s.Mode = m
	f.setModeCalls = append(f.setModeCalls, m)
	return nil
}
func (f *spyThermostatService) SetFanSpeed(ff thermostat.FanSpeed) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.s.FanSpeed = ff
	f.setFanCalls = append(f.setFanCalls, ff)
	return nil
}

func findFreeTCPAddr(t *testing.T) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	a := l.Addr().String()
	_ = l.Close()
	return a
}

const SyncInterval = 50 * time.Millisecond

func TestModbusControllerHandlers(t *testing.T) {
	fs := &spyThermostatService{}
	fs.s = thermostat.Snapshot{
		Enabled:                true,
		TemperatureSetpoint:    22.5,
		TemperatureSetpointMin: 16.0,
		TemperatureSetpointMax: 28.0,
		Mode:                   thermostat.ModeAuto,
		FanSpeed:               thermostat.FanAuto,
		AmbientTemperature:     21.25,
	}

	addr := findFreeTCPAddr(t)

	ctrl, err := New(fs, Config{
		DeviceID:     "dev",
		Addr:         addr,
		UnitID:       1,
		SyncInterval: SyncInterval,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := t.Context()
	go func() {
		_ = ctrl.Run(ctx)
	}()

	time.Sleep(SyncInterval)

	handler := modbus.NewTCPClientHandler(addr)
	if err := handler.Connect(); err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer handler.Close()
	client := modbus.NewClient(handler)

	// Read holding registers 0..4
	res, err := client.ReadHoldingRegisters(0, 5)
	if err != nil {
		t.Fatalf("read holding: %v", err)
	}
	if len(res) != 10 {
		t.Fatalf("expected 10 bytes got %d", len(res))
	}
	get := func(i int) uint16 { return binary.BigEndian.Uint16(res[i*2 : i*2+2]) }
	if get(0) != encodeTemp(fs.s.TemperatureSetpoint) {
		t.Fatalf("setpoint mismatch")
	}
	if get(3) != uint16(fs.s.Mode) {
		t.Fatalf("mode mismatch")
	}

	// Write setpoint register
	newSP := encodeTemp(25.75)
	if _, err := client.WriteSingleRegister(0, newSP); err != nil {
		t.Fatalf("write register: %v", err)
	}
	// allow sync to run
	time.Sleep(SyncInterval)
	fs.mu.Lock()
	if len(fs.setSetpointCalls) == 0 || fs.setSetpointCalls[len(fs.setSetpointCalls)-1] != decodeTemp(newSP) {
		fs.mu.Unlock()
		t.Fatalf("setSetpoint not called")
	}
	fs.mu.Unlock()

	// Write coil 0 disabled
	if _, err := client.WriteSingleCoil(0, 0x0000); err != nil {
		t.Fatalf("write coil: %v", err)
	}
	time.Sleep(SyncInterval)
	fs.mu.Lock()
	if len(fs.setEnabledCalls) == 0 || fs.setEnabledCalls[len(fs.setEnabledCalls)-1] != false {
		fs.mu.Unlock()
		t.Fatalf("setEnabled not called")
	}
	fs.mu.Unlock()
}
