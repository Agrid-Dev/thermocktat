package thermostat

import "context"

// Service is the inbound (driving) port: the thermostat's control API,
// implemented by *Thermostat and consumed by the controllers
// (http/mqtt/modbus/bacnet/knx). It is the key decoupling boundary — keep new
// controllers depending on this interface, not on the concrete *Thermostat.
type Service interface {
	Get() Snapshot
	SetEnabled(bool)
	SetSetpoint(float64) error
	SetMinMax(min, max float64) error
	SetMode(Mode) error
	SetFanSpeed(FanSpeed) error
	SetFaultCode(int)
}

// WeatherProvider is the outbound (driven) port: the outdoor temperature the
// heat-loss simulation needs, implemented by adapters in internal/weather.
type WeatherProvider interface {
	OutdoorTemperature(ctx context.Context) (float64, error)
}

// The core implements its own inbound port.
var _ Service = (*Thermostat)(nil)
