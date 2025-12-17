package ports

import "github.com/Agrid-Dev/thermocktat/internal/thermostat"

// ThermostatService is the control-plane port used by controllers (HTTP/MQTT/etc).
type ThermostatService interface {
	Get() thermostat.Snapshot
	SetEnabled(bool)
	SetSetpoint(float64) error
	SetMinMax(min, max float64) error
	SetMode(thermostat.Mode) error
	SetFanSpeed(thermostat.FanSpeed) error
}
