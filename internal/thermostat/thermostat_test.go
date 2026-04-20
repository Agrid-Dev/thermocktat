package thermostat

import (
	"math"
	"testing"
	"time"
)

func assertError(t *testing.T, err error, expected error) {
	t.Helper()
	if err != expected {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}

func newTestSnapshot(opts ...func(*Snapshot)) Snapshot {
	s := Snapshot{
		Enabled:                true,
		TemperatureSetpoint:    22,
		TemperatureSetpointMin: 16,
		TemperatureSetpointMax: 28,
		Mode:                   ModeAuto,
		FanSpeed:               FanAuto,
		AmbientTemperature:     21,
	}

	for _, opt := range opts {
		opt(&s)
	}

	return s
}

func newTestThermostat(t *testing.T, pidParams PIDRegulatorParams, heatLossParams HeatLossSimulatorParams, opts ...func(*Snapshot)) *Thermostat {
	t.Helper()

	s := newTestSnapshot()

	for _, opt := range opts {
		opt(&s)
	}

	th, err := New(nil, s, pidParams, heatLossParams)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	return th
}

func TestNewValidationInvalidMinMax(t *testing.T) {
	s := newTestSnapshot(func(s *Snapshot) {
		s.TemperatureSetpointMin = 28
		s.TemperatureSetpointMax = 16
	})

	_, err := New(nil, s, PIDRegulatorParams{}, HeatLossSimulatorParams{})
	assertError(t, err, ErrInvalidMinMax)
}

func TestNewValidationInvalidFanSpeed(t *testing.T) {
	s := newTestSnapshot(func(s *Snapshot) {
		s.FanSpeed = FanSpeed(999)
	})
	_, err := New(nil, s, PIDRegulatorParams{}, HeatLossSimulatorParams{})
	assertError(t, err, ErrInvalidFanSpeed)
}

func TestNewValidationInvaliMode(t *testing.T) {
	s := newTestSnapshot(func(s *Snapshot) {
		s.Mode = Mode(999)
	})
	_, err := New(nil, s, PIDRegulatorParams{}, HeatLossSimulatorParams{})
	assertError(t, err, ErrInvalidMode)
}

func TestNewValidationInvalidSetpoint(t *testing.T) {
	s := newTestSnapshot(func(s *Snapshot) {
		s.TemperatureSetpoint = 4
	})
	_, err := New(nil, s, PIDRegulatorParams{}, HeatLossSimulatorParams{})
	assertError(t, err, ErrSetpointOutOfRange)
}

func TestModeValidation(t *testing.T) {
	th := newTestThermostat(t, PIDRegulatorParams{}, HeatLossSimulatorParams{})
	if err := th.SetMode(Mode(999)); err != ErrInvalidMode {
		t.Fatalf("expected ErrInvalidMode, got %v", err)
	}
}

func TestFanValidation(t *testing.T) {
	th := newTestThermostat(t, PIDRegulatorParams{}, HeatLossSimulatorParams{})
	if err := th.SetFanSpeed(FanSpeed(999)); err != ErrInvalidFanSpeed {
		t.Fatalf("expected ErrInvalidFanSpeed, got %v", err)
	}
}

func TestSetMinMaxKeepsSetpointValid(t *testing.T) {
	th := newTestThermostat(t, PIDRegulatorParams{}, HeatLossSimulatorParams{})
	if err := th.SetMinMax(23, 28); err != ErrSetpointOutOfRange {
		t.Fatalf("expected ErrSetpointOutOfRange, got %v", err)
	}
}

func assertEqual[T comparable](t *testing.T, name string, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: got %v, want %v", name, got, want)
	}
}

func TestSetEnabled(t *testing.T) {
	th := newTestThermostat(t, PIDRegulatorParams{}, HeatLossSimulatorParams{}, func(s *Snapshot) {
		s.Enabled = false
	})
	th.SetEnabled(true)
	assertEqual(t, "enabled", th.Get().Enabled, true)
}

func TestEnable(t *testing.T) {
	th := newTestThermostat(t, PIDRegulatorParams{}, HeatLossSimulatorParams{}, func(s *Snapshot) {
		s.Enabled = false
	})
	th.Enable()
	assertEqual(t, "enabled", th.Get().Enabled, true)
}

func TestDisable(t *testing.T) {
	th := newTestThermostat(t, PIDRegulatorParams{}, HeatLossSimulatorParams{})
	th.Disable()
	assertEqual(t, "enabled", th.Get().Enabled, false)
}

func TestSetSetpoint(t *testing.T) {
	th := newTestThermostat(t, PIDRegulatorParams{}, HeatLossSimulatorParams{})
	err := th.SetSetpoint(25.5)
	assertEqual(t, "setpoint", th.Get().TemperatureSetpoint, 25.5)
	assertError(t, err, nil)
}

func TestSetpointBounds(t *testing.T) {
	th := newTestThermostat(t, PIDRegulatorParams{}, HeatLossSimulatorParams{})
	if err := th.SetSetpoint(15.9); err != nil {
		assertError(t, err, ErrSetpointOutOfRange)
	}
	if err := th.SetSetpoint(28.1); err != nil {
		assertError(t, err, ErrSetpointOutOfRange)
	}
	assertError(t, th.SetSetpoint(16.0), nil)
	assertError(t, th.SetSetpoint(28.0), nil)
}

func TestSetMode(t *testing.T) {
	th := newTestThermostat(t, PIDRegulatorParams{}, HeatLossSimulatorParams{})
	err := th.SetMode(ModeHeat)
	assertError(t, err, nil)
	assertEqual(t, "mode", th.Get().Mode, ModeHeat)
}

func TestSetMinMax(t *testing.T) {
	th := newTestThermostat(t, PIDRegulatorParams{}, HeatLossSimulatorParams{})
	err := th.SetMinMax(12.0, 30.0)
	assertError(t, err, nil)
	assertEqual(t, "min", th.Get().TemperatureSetpointMin, 12.0)
	assertEqual(t, "max", th.Get().TemperatureSetpointMax, 30.0)
}

func TestSetMinMaxInvalid(t *testing.T) {
	th := newTestThermostat(t, PIDRegulatorParams{}, HeatLossSimulatorParams{})
	err := th.SetMinMax(25., 20.0)
	assertError(t, err, ErrInvalidMinMax)
}

func TestSetFanSpeed(t *testing.T) {
	th := newTestThermostat(t, PIDRegulatorParams{}, HeatLossSimulatorParams{})
	err := th.SetFanSpeed(FanHigh)
	assertEqual(t, "FanSpeed", th.Get().FanSpeed, FanHigh)
	assertError(t, err, nil)
}

func TestSetFaultCode(t *testing.T) {
	th := newTestThermostat(t, PIDRegulatorParams{}, HeatLossSimulatorParams{})
	assertEqual(t, "FaultCode default", th.Get().FaultCode, 0)
	th.SetFaultCode(42)
	assertEqual(t, "FaultCode", th.Get().FaultCode, 42)
}

func TestSetAmbient(t *testing.T) {
	th := newTestThermostat(t, PIDRegulatorParams{}, HeatLossSimulatorParams{})
	th.setAmbient(25.4)
	assertEqual(t, "AmbientTemperature", th.Get().AmbientTemperature, 25.4)
}

func TestDisabledNoRegulation(t *testing.T) {
	outdoorTemp := 10.0
	heatLossCoeff := 0.001
	initialAmbient := 21.0
	dt := 1 * time.Second

	pidParams := PIDRegulatorParams{
		Kp:                   1.0,
		Ki:                   0.1,
		Kd:                   0.01,
		TargetHysteresis:     1.0,
		ModeChangeHysteresis: 2.0,
	}
	heatLossParams := HeatLossSimulatorParams{
		OutdoorTemperature: outdoorTemp,
		Coefficient:        heatLossCoeff,
	}

	th := newTestThermostat(t, pidParams, heatLossParams, func(s *Snapshot) {
		s.Enabled = false
		s.AmbientTemperature = initialAmbient
		s.Mode = ModeHeat
		s.TemperatureSetpoint = 25 // setpoint above ambient => would heat if enabled
	})

	th.UpdateAmbient(dt)

	// Expected: only heat loss, no regulation
	expectedDelta := heatLossCoeff * (outdoorTemp - initialAmbient) * dt.Seconds()
	expectedAmbient := initialAmbient + expectedDelta
	got := th.Get().AmbientTemperature

	if math.Abs(got-expectedAmbient) > 1e-9 {
		t.Fatalf("disabled thermostat should only apply heat loss: got %.9f, want %.9f", got, expectedAmbient)
	}
}
