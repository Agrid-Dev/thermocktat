package thermostat

import "fmt"

// Mode is an integer enum.
type Mode int

const (
	ModeUnknown Mode = iota
	ModeHeat
	ModeCool
	ModeFan
	ModeAuto
)

func (m Mode) Valid() bool {
	return m == ModeHeat || m == ModeCool || m == ModeFan || m == ModeAuto
}

func (m Mode) String() string {
	switch m {
	case ModeHeat:
		return "heat"
	case ModeCool:
		return "cool"
	case ModeFan:
		return "fan"
	case ModeAuto:
		return "auto"
	default:
		return "unknown"
	}
}

// ParseMode is optional but handy for env vars / CLI.
func ParseMode(s string) (Mode, error) {
	switch s {
	case "heat":
		return ModeHeat, nil
	case "cool":
		return ModeCool, nil
	case "fan":
		return ModeFan, nil
	case "auto":
		return ModeAuto, nil
	default:
		return ModeUnknown, fmt.Errorf("invalid mode: %q", s)
	}
}

// FanSpeed is an integer enum.
type FanSpeed int

const (
	FanUnknown FanSpeed = iota
	FanAuto
	FanLow
	FanMedium
	FanHigh
)

func (f FanSpeed) Valid() bool {
	return f == FanAuto || f == FanLow || f == FanMedium || f == FanHigh
}

func (f FanSpeed) String() string {
	switch f {
	case FanAuto:
		return "auto"
	case FanLow:
		return "low"
	case FanMedium:
		return "medium"
	case FanHigh:
		return "high"
	default:
		return "unknown"
	}
}

func ParseFanSpeed(s string) (FanSpeed, error) {
	switch s {
	case "auto":
		return FanAuto, nil
	case "low":
		return FanLow, nil
	case "medium":
		return FanMedium, nil
	case "high":
		return FanHigh, nil
	default:
		return FanUnknown, fmt.Errorf("invalid fan speed: %q", s)
	}
}
