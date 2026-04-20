package modbusctrl

import (
	"encoding/binary"
	"math"
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
	setEnabledCalls   []bool
	setSetpointCalls  []float64
	setMinMaxCalls    [][2]float64
	setModeCalls      []thermostat.Mode
	setFanCalls       []thermostat.FanSpeed
	setFaultCodeCalls []int
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
func (f *spyThermostatService) SetFaultCode(code int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.s.FaultCode = code
	f.setFaultCodeCalls = append(f.setFaultCodeCalls, code)
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

	ctrl, err := New(nil, fs, Config{
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

	// Read holding registers 0..hrTotal-1
	res, err := client.ReadHoldingRegisters(0, hrTotal)
	if err != nil {
		t.Fatalf("read holding: %v", err)
	}
	if len(res) != 2*hrTotal {
		t.Fatalf("expected %d bytes got %d", 2*hrTotal, len(res))
	}
	get := func(i int) uint16 { return binary.BigEndian.Uint16(res[i*2 : i*2+2]) }
	if get(hrSetpoint) != encodeTemp(fs.s.TemperatureSetpoint) {
		t.Fatalf("setpoint mismatch: got %d want %d", get(hrSetpoint), encodeTemp(fs.s.TemperatureSetpoint))
	}
	if get(hrSetpointMin) != encodeTemp(fs.s.TemperatureSetpointMin) {
		t.Fatalf("setpoint_min mismatch")
	}
	if get(hrSetpointMax) != encodeTemp(fs.s.TemperatureSetpointMax) {
		t.Fatalf("setpoint_max mismatch")
	}
	if get(hrMode) != uint16(fs.s.Mode) {
		t.Fatalf("mode mismatch")
	}
	if get(hrFanSpeed) != uint16(fs.s.FanSpeed) {
		t.Fatalf("fan_speed mismatch")
	}
	if get(hrFaultCode) != uint16(fs.s.FaultCode) {
		t.Fatalf("fault_code mismatch")
	}

	// Write setpoint via function 6 (register address hrSetpoint=0)
	newSP := encodeTemp(25.75)
	if _, err := client.WriteSingleRegister(hrSetpoint, newSP); err != nil {
		t.Fatalf("write register: %v", err)
	}
	time.Sleep(SyncInterval)
	fs.mu.Lock()
	if len(fs.setSetpointCalls) == 0 || fs.setSetpointCalls[len(fs.setSetpointCalls)-1] != decodeTemp(newSP) {
		fs.mu.Unlock()
		t.Fatalf("setSetpoint not called")
	}
	fs.mu.Unlock()

	// Write mode via function 6 (register address hrMode=6)
	if _, err := client.WriteSingleRegister(hrMode, uint16(thermostat.ModeHeat)); err != nil {
		t.Fatalf("write mode: %v", err)
	}
	time.Sleep(SyncInterval)
	fs.mu.Lock()
	if len(fs.setModeCalls) == 0 || fs.setModeCalls[len(fs.setModeCalls)-1] != thermostat.ModeHeat {
		fs.mu.Unlock()
		t.Fatalf("setMode not called")
	}
	fs.mu.Unlock()

	// Write fault_code via function 6
	if _, err := client.WriteSingleRegister(hrFaultCode, 42); err != nil {
		t.Fatalf("write fault_code: %v", err)
	}
	time.Sleep(SyncInterval)
	fs.mu.Lock()
	if len(fs.setFaultCodeCalls) == 0 || fs.setFaultCodeCalls[len(fs.setFaultCodeCalls)-1] != 42 {
		fs.mu.Unlock()
		t.Fatalf("setFaultCode not called")
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

	// Read input registers (ambient temperature)
	irRes, err := client.ReadInputRegisters(irAmbient, 1)
	if err != nil {
		t.Fatalf("read input registers: %v", err)
	}
	if len(irRes) != 2 {
		t.Fatalf("expected 2 bytes got %d", len(irRes))
	}
	irVal := binary.BigEndian.Uint16(irRes[0:2])
	if irVal != encodeTemp(fs.s.AmbientTemperature) {
		t.Fatalf("ambient temp mismatch: got %d want %d", irVal, encodeTemp(fs.s.AmbientTemperature))
	}
}

func TestModbusController32Bit(t *testing.T) {
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

	ctrl, err := New(nil, fs, Config{
		DeviceID:      "dev",
		Addr:          addr,
		UnitID:        1,
		SyncInterval:  SyncInterval,
		RegisterCount: 2,
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

	// Read all holding registers
	res, err := client.ReadHoldingRegisters(0, 10)
	if err != nil {
		t.Fatalf("read holding: %v", err)
	}
	if len(res) != 20 {
		t.Fatalf("expected 20 bytes got %d", len(res))
	}
	get := func(i int) uint16 { return binary.BigEndian.Uint16(res[i*2 : i*2+2]) }

	// Verify setpoint is encoded as float32 across 2 registers
	spBits := uint32(get(hrSetpoint))<<16 | uint32(get(hrSetpoint+1))
	spFloat := math.Float32frombits(spBits)
	if spFloat != float32(22.5) {
		t.Fatalf("32-bit setpoint mismatch: got %f want %f", spFloat, float32(22.5))
	}

	// Mode should still be plain uint16
	if get(hrMode) != uint16(thermostat.ModeAuto) {
		t.Fatalf("mode mismatch: got %d want %d", get(hrMode), uint16(thermostat.ModeAuto))
	}

	// Function 6 (write single register) should fail for temperature addresses in 32-bit mode
	if _, err := client.WriteSingleRegister(hrSetpoint, 2000); err == nil {
		t.Fatalf("expected error writing single temp register in 32-bit mode")
	}

	// Function 6 should still work for mode
	if _, err := client.WriteSingleRegister(hrMode, uint16(thermostat.ModeCool)); err != nil {
		t.Fatalf("write mode in 32-bit mode: %v", err)
	}
	time.Sleep(SyncInterval)
	fs.mu.Lock()
	if len(fs.setModeCalls) == 0 || fs.setModeCalls[len(fs.setModeCalls)-1] != thermostat.ModeCool {
		fs.mu.Unlock()
		t.Fatalf("setMode not called correctly")
	}
	fs.mu.Unlock()

	// Write temperature via function 16 (write multiple registers) with float32 encoding
	newSP := float32(25.75)
	spU32 := math.Float32bits(newSP)
	writeData := make([]byte, 4)
	binary.BigEndian.PutUint16(writeData[0:2], uint16(spU32>>16))
	binary.BigEndian.PutUint16(writeData[2:4], uint16(spU32&0xFFFF))
	if _, err := client.WriteMultipleRegisters(hrSetpoint, 2, writeData); err != nil {
		t.Fatalf("write multiple registers: %v", err)
	}
	time.Sleep(SyncInterval)
	fs.mu.Lock()
	if len(fs.setSetpointCalls) == 0 {
		fs.mu.Unlock()
		t.Fatalf("setSetpoint not called")
	}
	gotSP := fs.setSetpointCalls[len(fs.setSetpointCalls)-1]
	fs.mu.Unlock()
	if math.Abs(gotSP-float64(newSP)) > 0.001 {
		t.Fatalf("setSetpoint value mismatch: got %f want %f", gotSP, newSP)
	}

	// Read input registers in 32-bit mode (2 registers for ambient temp)
	irRes, err := client.ReadInputRegisters(irAmbient, 2)
	if err != nil {
		t.Fatalf("read input registers: %v", err)
	}
	if len(irRes) != 4 {
		t.Fatalf("expected 4 bytes got %d", len(irRes))
	}
	ambBits := uint32(binary.BigEndian.Uint16(irRes[0:2]))<<16 | uint32(binary.BigEndian.Uint16(irRes[2:4]))
	ambFloat := math.Float32frombits(ambBits)
	if ambFloat != float32(21.25) {
		t.Fatalf("32-bit ambient mismatch: got %f want %f", ambFloat, float32(21.25))
	}
}
