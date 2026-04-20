package thermostat

import (
	"context"
	"log/slog"
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
	FaultCode              int
}

type Thermostat struct {
	mu       sync.RWMutex
	s        Snapshot
	reg      PIDRegulator
	heatLoss HeatLossSimulator
	log      *slog.Logger
}

func New(logger *slog.Logger, initial Snapshot, pidParams PIDRegulatorParams, heatLossParams HeatLossSimulatorParams) (*Thermostat, error) {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	t := &Thermostat{log: logger}
	if err := validateSnapshot(initial); err != nil {
		return nil, err
	}
	t.s = initial
	t.reg = *NewPIDRegulator(pidParams)
	heatLoss, err := NewHeatLossSimulator(heatLossParams)
	if err != nil {
		return nil, err
	}
	t.heatLoss = *heatLoss
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
	prev := t.s.Enabled
	t.s.Enabled = on
	t.mu.Unlock()
	if prev != on {
		t.log.Info("enabled changed", "from", prev, "to", on)
	}
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
	prev := t.s.Mode
	t.s.Mode = m
	t.mu.Unlock()
	if prev != m {
		t.log.Info("mode changed", "from", prev.String(), "to", m.String())
	}
	return nil
}

func (t *Thermostat) SetFanSpeed(f FanSpeed) error {
	if !f.Valid() {
		return ErrInvalidFanSpeed
	}
	t.mu.Lock()
	prev := t.s.FanSpeed
	t.s.FanSpeed = f
	t.mu.Unlock()
	if prev != f {
		t.log.Info("fan_speed changed", "from", prev.String(), "to", f.String())
	}
	return nil
}

func (t *Thermostat) SetFaultCode(code int) {
	t.mu.Lock()
	prev := t.s.FaultCode
	t.s.FaultCode = code
	t.mu.Unlock()
	if prev != code {
		t.log.Info("fault_code changed", "from", prev, "to", code)
	}
}

func (t *Thermostat) SetMinMax(min, max float64) error {
	if min > max {
		return ErrInvalidMinMax
	}

	t.mu.Lock()
	// Enforce current setpoint remains valid
	if t.s.TemperatureSetpoint < min || t.s.TemperatureSetpoint > max {
		t.mu.Unlock()
		return ErrSetpointOutOfRange
	}
	prevMin, prevMax := t.s.TemperatureSetpointMin, t.s.TemperatureSetpointMax
	t.s.TemperatureSetpointMin = min
	t.s.TemperatureSetpointMax = max
	t.mu.Unlock()
	if prevMin != min || prevMax != max {
		t.log.Info("setpoint bounds changed", "min", min, "max", max)
	}
	return nil
}

func (t *Thermostat) SetSetpoint(sp float64) error {
	t.mu.Lock()
	if sp < t.s.TemperatureSetpointMin || sp > t.s.TemperatureSetpointMax {
		t.mu.Unlock()
		return ErrSetpointOutOfRange
	}
	prev := t.s.TemperatureSetpoint
	t.s.TemperatureSetpoint = sp
	t.mu.Unlock()
	if prev != sp {
		t.log.Info("setpoint changed", "from", prev, "to", sp)
	}
	return nil
}

// Internal: used by simulator
func (t *Thermostat) setAmbient(temp float64) {
	// lock held by caller
	t.s.AmbientTemperature = temp
}

func (t *Thermostat) UpdateAmbient(dt time.Duration) {
	t.mu.Lock()
	prevH, prevC := t.reg.activation()
	var deltaReg float64
	if t.s.Enabled {
		deltaReg = t.reg.DeltaTemperature(t.s.TemperatureSetpoint, t.s.AmbientTemperature, t.s.Mode, dt)
	}
	deltaHeatLoss := t.heatLoss.DeltaTemperature(t.s.AmbientTemperature, dt)
	newAmbient := t.s.AmbientTemperature + deltaReg + deltaHeatLoss
	t.setAmbient(newAmbient)
	curH, curC := t.reg.activation()
	setpoint, ambient, mode := t.s.TemperatureSetpoint, t.s.AmbientTemperature, t.s.Mode
	t.mu.Unlock()

	if prevH != curH || prevC != curC {
		t.log.Info("regulation activation changed",
			"from", activationLabel(prevH, prevC),
			"to", activationLabel(curH, curC),
			"setpoint", setpoint,
			"ambient", ambient,
			"mode", mode.String(),
		)
	}
}

func activationLabel(heating, cooling bool) string {
	switch {
	case heating:
		return "heating"
	case cooling:
		return "cooling"
	default:
		return "off"
	}
}

func (t *Thermostat) Run(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			t.UpdateAmbient(interval)
		}
	}
}
