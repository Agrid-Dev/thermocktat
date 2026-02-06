package thermostat

import (
	"time"
)

type PIDRegulatorParams struct {
	Kp                   float64
	Ki                   float64
	Kd                   float64
	TargetHysteresis     float64 // hysteresis for regulation within a mode (target reached)
	ModeChangeHysteresis float64 // hysteresis for mode switch in auto mode (> TargetHysteresis)
}

func (params *PIDRegulatorParams) Validate() error {
	if !(params.ModeChangeHysteresis > params.TargetHysteresis) {
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

func (pid *PIDRegulator) setActivation(heating, cooling bool) {
	if pid.isHeating == heating && pid.isCooling == cooling {
		return
	}
	// Reset integrator and derivative memory on state change to avoid windup
	pid.integral = 0
	pid.prevError = 0
	pid.isHeating = heating
	pid.isCooling = cooling
}

func (pid *PIDRegulator) Activate(setpoint, ambient float64, mode Mode) {
	// Precompute commonly used thresholds
	lowTarget := setpoint - pid.params.TargetHysteresis
	highTarget := setpoint + pid.params.TargetHysteresis
	lowModeChange := setpoint - pid.params.ModeChangeHysteresis
	highModeChange := setpoint + pid.params.ModeChangeHysteresis

	// Fan mode turns everything off
	if mode == ModeFan {
		pid.setActivation(false, false)
		return
	}

	// Explicit Heat or Cool modes
	switch mode {
	case ModeHeat:
		if ambient < lowTarget {
			pid.setActivation(true, false)
			return
		}
	case ModeCool:
		if ambient > highTarget {
			pid.setActivation(false, true)
			return
		}
	case ModeAuto:
		// Possibly switch based on mode-change hysteresis
		if ambient > highModeChange {
			pid.setActivation(false, true)
			return
		}
		if ambient < lowModeChange {
			pid.setActivation(true, false)
			return
		}
	}

	// Stop heating/cooling when the target is reached (target hysteresis)
	if pid.isHeating && ambient >= highTarget {
		pid.setActivation(false, false)
	} else if pid.isCooling && ambient <= lowTarget {
		pid.setActivation(false, false)
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

func (pid *PIDRegulator) DeltaTemperature(setpoint, ambient float64, mode Mode, dt time.Duration) float64 {
	pid.Activate(setpoint, ambient, mode)
	target := pid.GetTarget(setpoint, ambient, mode)
	error := target - ambient

	// Apply PID control if heating or cooling
	if pid.isHeating || pid.isCooling {
		pid.integral += error * dt.Seconds()
		derivative := (error - pid.prevError) / dt.Seconds()
		pid.prevError = error

		output := pid.params.Kp*error + pid.params.Ki*pid.integral + pid.params.Kd*derivative
		return output
	}

	return 0
}
