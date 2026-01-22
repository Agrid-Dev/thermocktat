package thermostat

import (
	"context"
	"sync"
	"time"
)

type Snapshot struct {
	Enabled                bool
	TemperatureSetpoint    float64
	TemperatureSetpointMin float64
	TemperatureSetpointMax float64
	Mode                   Mode
	FanSpeed               FanSpeed
	AmbientTemperature     float64
}

type Thermostat struct {
	mu  sync.RWMutex
	s   Snapshot
	reg PIDRegulator
}

func New(initial Snapshot, pidParams PIDRegulatorParams) (*Thermostat, error) {
	t := &Thermostat{}
	if err := validateSnapshot(initial); err != nil {
		return nil, err
	}
	t.s = initial
	t.reg = *NewPIDRegulator(pidParams)
	return t, nil
}

func validateSnapshot(s Snapshot) error {
	if !s.Mode.Valid() {
		return ErrInvalidMode
	}
	if !s.FanSpeed.Valid() {
		return ErrInvalidFanSpeed
	}
	if s.TemperatureSetpointMin > s.TemperatureSetpointMax {
		return ErrInvalidMinMax
	}
	if s.TemperatureSetpoint < s.TemperatureSetpointMin || s.TemperatureSetpoint > s.TemperatureSetpointMax {
		return ErrSetpointOutOfRange
	}
	return nil
}

func (t *Thermostat) Get() Snapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.s
}

func (t *Thermostat) SetEnabled(on bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.s.Enabled = on
}

func (t *Thermostat) Enable() {
	t.SetEnabled(true)
}

func (t *Thermostat) Disable() {
	t.SetEnabled(false)
}

func (t *Thermostat) SetMode(m Mode) error {
	if !m.Valid() {
		return ErrInvalidMode
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.s.Mode = m
	return nil
}

func (t *Thermostat) SetFanSpeed(f FanSpeed) error {
	if !f.Valid() {
		return ErrInvalidFanSpeed
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.s.FanSpeed = f
	return nil
}

func (t *Thermostat) SetMinMax(min, max float64) error {
	if min > max {
		return ErrInvalidMinMax
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Enforce current setpoint remains valid
	if t.s.TemperatureSetpoint < min || t.s.TemperatureSetpoint > max {
		return ErrSetpointOutOfRange
	}

	t.s.TemperatureSetpointMin = min
	t.s.TemperatureSetpointMax = max
	return nil
}

func (t *Thermostat) SetSetpoint(sp float64) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if sp < t.s.TemperatureSetpointMin || sp > t.s.TemperatureSetpointMax {
		return ErrSetpointOutOfRange
	}
	t.s.TemperatureSetpoint = sp
	return nil
}

// Internal: used by simulator
func (t *Thermostat) setAmbient(temp float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.s.AmbientTemperature = temp
}

func (t *Thermostat) Run(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			t.UpdateAmbientTemperature()
		}
	}
}
