package thermostat

type PIDRegulatorParams struct {
	Kp         float64
	Ki         float64
	Kd         float64
	Hysteresis float64
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
	if (mode == ModeHeat || mode == ModeAuto) && ambient < setpoint-pid.params.Hysteresis && !pid.isHeating {
		pid.isHeating = true
		pid.isCooling = false
	} else if (mode == ModeCool || mode == ModeAuto) && ambient > setpoint+pid.params.Hysteresis && !pid.isCooling {
		pid.isCooling = true
		pid.isHeating = false
	}
	// Check if we need to stop heating or cooling (target reached)
	if pid.isHeating && ambient >= setpoint+pid.params.Hysteresis {
		pid.isHeating = false
	} else if pid.isCooling && ambient <= setpoint-pid.params.Hysteresis {
		pid.isCooling = false
	}

}

func (pid *PIDRegulator) GetTarget(setpoint, ambient float64, mode Mode) float64 {
	if pid.isHeating {
		return setpoint + pid.params.Hysteresis
	}
	if pid.isCooling {
		return setpoint - pid.params.Hysteresis
	}
	return setpoint
}

func (pid *PIDRegulator) Update(setpoint, ambient float64, mode Mode) float64 {
	pid.Activate(setpoint, ambient, mode)
	target := pid.GetTarget(setpoint, ambient, mode)
	error := target - ambient

	// Apply PID control if heating or cooling
	if pid.isHeating || pid.isCooling {
		pid.integral += error
		derivative := error - pid.prevError
		pid.prevError = error

		output := pid.params.Kp*error + pid.params.Ki*pid.integral + pid.params.Kd*derivative
		return ambient + output
	}

	return ambient
}

func (t *Thermostat) UpdateAmbientTemperature() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.s.AmbientTemperature = t.reg.Update(t.s.TemperatureSetpoint, t.s.AmbientTemperature, t.s.Mode)
}
