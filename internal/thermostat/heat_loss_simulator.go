package thermostat

import "time"

type HeatLossSimulatorParams struct {
	OutdoorTemperature float64
	Coefficient        float64 // >= 0, represents conductivity. 0 for no loss.
}

func (params *HeatLossSimulatorParams) Validate() error {
	if params.Coefficient < 0 {
		return ErrNegativeHeatLossCoefficient
	}
	return nil
}

type HeatLossSimulator struct {
	params HeatLossSimulatorParams
}

func NewHeatLossSimulator(params HeatLossSimulatorParams) (*HeatLossSimulator, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}
	return &HeatLossSimulator{params: params}, nil
}

func (heatLoss *HeatLossSimulator) DeltaTemperature(indoorTemperature float64, dt time.Duration) float64 {
	diff := heatLoss.params.OutdoorTemperature - indoorTemperature
	return heatLoss.params.Coefficient * diff * dt.Seconds()
}
