package ports

import "context"

// WeatherProvider is the outbound port supplying the outdoor temperature (°C)
// for the heat-loss simulation.
type WeatherProvider interface {
	OutdoorTemperature(ctx context.Context) (float64, error)
}
