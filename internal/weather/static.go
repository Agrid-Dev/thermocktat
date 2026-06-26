// Package weather provides thermostat.WeatherProvider implementations: a fixed
// static value and a dynamic Open-Meteo client.
package weather

import (
	"context"

	"github.com/Agrid-Dev/thermocktat/internal/thermostat"
)

var (
	_ thermostat.WeatherProvider = (*Static)(nil)
	_ thermostat.WeatherProvider = (*OpenMeteo)(nil)
)

// Static always returns the same outdoor temperature.
type Static struct {
	temperature float64
}

func NewStatic(temperature float64) *Static {
	return &Static{temperature: temperature}
}

func (s *Static) OutdoorTemperature(context.Context) (float64, error) {
	return s.temperature, nil
}
