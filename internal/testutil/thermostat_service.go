package testutil

import "github.com/Agrid-Dev/thermocktat/internal/thermostat"

// FakeThermostatService is a reusable fake implementing ports.ThermostatService.
// Put ONLY what multiple test packages need here.
type FakeThermostatService struct {
	S thermostat.Snapshot

	SetEnabledCalled bool
	SetEnabledArg    bool

	SetSetpointCalled bool
	SetSetpointArg    float64
	SetSetpointErr    error

	SetMinMaxCalled bool
	SetMinMaxMin    float64
	SetMinMaxMax    float64
	SetMinMaxErr    error

	SetModeCalled bool
	SetModeArg    thermostat.Mode
	SetModeErr    error

	SetFanSpeedCalled bool
	SetFanSpeedArg    thermostat.FanSpeed
	SetFanSpeedErr    error
}

func NewFakeThermostatService() *FakeThermostatService {
	return &FakeThermostatService{
		S: thermostat.Snapshot{
			Enabled:                true,
			TemperatureSetpoint:    22,
			TemperatureSetpointMin: 16,
			TemperatureSetpointMax: 28,
			Mode:                   thermostat.ModeAuto,
			FanSpeed:               thermostat.FanAuto,
			AmbientTemperature:     21,
		},
	}
}

func (f *FakeThermostatService) Get() thermostat.Snapshot { return f.S }

func (f *FakeThermostatService) SetEnabled(b bool) {
	f.SetEnabledCalled = true
	f.SetEnabledArg = b
	f.S.Enabled = b
}

func (f *FakeThermostatService) SetSetpoint(v float64) error {
	f.SetSetpointCalled = true
	f.SetSetpointArg = v
	if f.SetSetpointErr != nil {
		return f.SetSetpointErr
	}
	f.S.TemperatureSetpoint = v
	return nil
}

func (f *FakeThermostatService) SetMinMax(min, max float64) error {
	f.SetMinMaxCalled = true
	f.SetMinMaxMin = min
	f.SetMinMaxMax = max
	if f.SetMinMaxErr != nil {
		return f.SetMinMaxErr
	}
	f.S.TemperatureSetpointMin = min
	f.S.TemperatureSetpointMax = max
	return nil
}

func (f *FakeThermostatService) SetMode(m thermostat.Mode) error {
	f.SetModeCalled = true
	f.SetModeArg = m
	if f.SetModeErr != nil {
		return f.SetModeErr
	}
	f.S.Mode = m
	return nil
}

func (f *FakeThermostatService) SetFanSpeed(fs thermostat.FanSpeed) error {
	f.SetFanSpeedCalled = true
	f.SetFanSpeedArg = fs
	if f.SetFanSpeedErr != nil {
		return f.SetFanSpeedErr
	}
	f.S.FanSpeed = fs
	return nil
}
