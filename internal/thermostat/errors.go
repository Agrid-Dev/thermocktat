package thermostat

import "errors"

var (
	ErrInvalidMode        = errors.New("invalid mode")
	ErrInvalidFanSpeed    = errors.New("invalid fan speed")
	ErrInvalidSetpoint    = errors.New("invalid temperature setpoint")
	ErrInvalidMinMax      = errors.New("invalid min/max setpoints")
	ErrSetpointOutOfRange = errors.New("setpoint out of range")
)
