package thermostat

import "testing"

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

func newTestThermostat(t *testing.T, pidParams PIDRegulatorParams, opts ...func(*Snapshot)) *Thermostat {
	t.Helper()

	s := newTestSnapshot()

	for _, opt := range opts {
		opt(&s)
	}

	th, err := New(s, pidParams)
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

	_, err := New(s, PIDRegulatorParams{})
	assertError(t, err, ErrInvalidMinMax)
}

func TestNewValidationInvalidFanSpeed(t *testing.T) {
	s := newTestSnapshot(func(s *Snapshot) {
		s.FanSpeed = FanSpeed(999)
	})
	_, err := New(s, PIDRegulatorParams{})
	assertError(t, err, ErrInvalidFanSpeed)
}

func TestNewValidationInvaliMode(t *testing.T) {
	s := newTestSnapshot(func(s *Snapshot) {
		s.Mode = Mode(999)
	})
	_, err := New(s, PIDRegulatorParams{})
	assertError(t, err, ErrInvalidMode)
}

func TestNewValidationInvalidSetpoint(t *testing.T) {
	s := newTestSnapshot(func(s *Snapshot) {
		s.TemperatureSetpoint = 4
	})
	_, err := New(s, PIDRegulatorParams{})
	assertError(t, err, ErrSetpointOutOfRange)
}

func TestModeValidation(t *testing.T) {
	th := newTestThermostat(t, PIDRegulatorParams{})
	if err := th.SetMode(Mode(999)); err != ErrInvalidMode {
		t.Fatalf("expected ErrInvalidMode, got %v", err)
	}
}

func TestFanValidation(t *testing.T) {
	th := newTestThermostat(t, PIDRegulatorParams{})
	if err := th.SetFanSpeed(FanSpeed(999)); err != ErrInvalidFanSpeed {
		t.Fatalf("expected ErrInvalidFanSpeed, got %v", err)
	}
}

func TestSetMinMaxKeepsSetpointValid(t *testing.T) {
	th := newTestThermostat(t, PIDRegulatorParams{})
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
	th := newTestThermostat(t, PIDRegulatorParams{}, func(s *Snapshot) {
		s.Enabled = false
	})
	th.SetEnabled(true)
	assertEqual(t, "enabled", th.Get().Enabled, true)
}

func TestEnable(t *testing.T) {
	th := newTestThermostat(t, PIDRegulatorParams{}, func(s *Snapshot) {
		s.Enabled = false
	})
	th.Enable()
	assertEqual(t, "enabled", th.Get().Enabled, true)
}

func TestDisable(t *testing.T) {
	th := newTestThermostat(t, PIDRegulatorParams{})
	th.Disable()
	assertEqual(t, "enabled", th.Get().Enabled, false)
}

func TestSetSetpoint(t *testing.T) {
	th := newTestThermostat(t, PIDRegulatorParams{})
	err := th.SetSetpoint(25.5)
	assertEqual(t, "setpoint", th.Get().TemperatureSetpoint, 25.5)
	assertError(t, err, nil)
}

func TestSetpointBounds(t *testing.T) {
	th := newTestThermostat(t, PIDRegulatorParams{})
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
	th := newTestThermostat(t, PIDRegulatorParams{})
	err := th.SetMode(ModeHeat)
	assertError(t, err, nil)
	assertEqual(t, "mode", th.Get().Mode, ModeHeat)
}

func TestSetMinMax(t *testing.T) {
	th := newTestThermostat(t, PIDRegulatorParams{})
	err := th.SetMinMax(12.0, 30.0)
	assertError(t, err, nil)
	assertEqual(t, "min", th.Get().TemperatureSetpointMin, 12.0)
	assertEqual(t, "max", th.Get().TemperatureSetpointMax, 30.0)
}

func TestSetMinMaxInvalid(t *testing.T) {
	th := newTestThermostat(t, PIDRegulatorParams{})
	err := th.SetMinMax(25., 20.0)
	assertError(t, err, ErrInvalidMinMax)
}

func TestSetFanSpeed(t *testing.T) {
	th := newTestThermostat(t, PIDRegulatorParams{})
	err := th.SetFanSpeed(FanHigh)
	assertEqual(t, "FanSpeed", th.Get().FanSpeed, FanHigh)
	assertError(t, err, nil)
}

func TestSetAmbient(t *testing.T) {
	th := newTestThermostat(t, PIDRegulatorParams{})
	th.setAmbient(25.4)
	assertEqual(t, "AmbientTemperature", th.Get().AmbientTemperature, 25.4)
}
