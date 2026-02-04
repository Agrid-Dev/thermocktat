package thermostat

import "time"

type PIDRegulatorParams struct {
	Kp                float64
	Ki                float64
	Kd                float64
	TriggerHysteresis float64 // hysteresis for heating / cooling start
	TargetHysteresis  float64 // hysteresis for heating / cooling stop (target reached)
}

func (params *PIDRegulatorParams) Validate() error {
	if params.TargetHysteresis > params.TriggerHysteresis {
		return ErrInvalidRegulatorHysteresis
	}
	if params.Kp < 0 || params.Ki < 0 || params.Kd < 0 {
		return ErrorInvalidRegulatorCoefficients
	}
	return nil
}

type PIDRegulator struct {
	params    PIDRegulatorParams
	prevError float64
	integral  float64
	isHeating bool
	isCooling bool
}

func NewPIDRegulator(params PIDRegulatorParams) *PIDRegulator {
	return &PIDRegulator{
		params: params,
	}
}

func (pid *PIDRegulator) Activate(setpoint, ambient float64, mode Mode) {
	if mode == ModeFan {
		pid.isCooling = false
		pid.isHeating = false
		return
	}
	if (mode == ModeHeat || mode == ModeAuto) && ambient < setpoint-pid.params.TriggerHysteresis && !pid.isHeating {
		pid.isHeating = true
		pid.isCooling = false
	} else if (mode == ModeCool || mode == ModeAuto) && ambient > setpoint+pid.params.TriggerHysteresis && !pid.isCooling {
		pid.isCooling = true
		pid.isHeating = false
	}
	// Check if we need to stop heating or cooling (target reached)
	if pid.isHeating && ambient >= setpoint+pid.params.TargetHysteresis {
		pid.isHeating = false
	} else if pid.isCooling && ambient <= setpoint-pid.params.TargetHysteresis {
		pid.isCooling = false
	}

}

func (pid *PIDRegulator) GetTarget(setpoint, ambient float64, mode Mode) float64 {
	if pid.isHeating {
		return setpoint + pid.params.TargetHysteresis
	}
	if pid.isCooling {
		return setpoint - pid.params.TargetHysteresis
	}
	return setpoint
}

func (pid *PIDRegulator) Update(setpoint, ambient float64, mode Mode, dt time.Duration) float64 {
	pid.Activate(setpoint, ambient, mode)
	target := pid.GetTarget(setpoint, ambient, mode)
	error := target - ambient

	// Apply PID control if heating or cooling
	if pid.isHeating || pid.isCooling {
		pid.integral += error * dt.Seconds()
		derivative := (error - pid.prevError) / dt.Seconds()
		pid.prevError = error

		output := pid.params.Kp*error + pid.params.Ki*pid.integral + pid.params.Kd*derivative
		return ambient + output
	}

	return ambient
}

func (t *Thermostat) UpdateAmbientTemperature(dt time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.s.AmbientTemperature = t.reg.Update(t.s.TemperatureSetpoint, t.s.AmbientTemperature, t.s.Mode, dt)
}
