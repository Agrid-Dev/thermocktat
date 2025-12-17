package device

import "github.com/Agrid-Dev/thermocktat/internal/thermostat"

type Device struct {
	ID string
	T  *thermostat.Thermostat
}

func New(id string, t *thermostat.Thermostat) *Device {
	return &Device{ID: id, T: t}
}
