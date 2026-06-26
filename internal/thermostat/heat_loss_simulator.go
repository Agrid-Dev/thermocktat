package thermostat

import (
	"sync"
	"time"
)

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
	mu          sync.RWMutex
	outdoorTemp float64
	coefficient float64
}

func NewHeatLossSimulator(params HeatLossSimulatorParams) (*HeatLossSimulator, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}
	return &HeatLossSimulator{
		outdoorTemp: params.OutdoorTemperature,
		coefficient: params.Coefficient,
	}, nil
}

// SetOutdoorTemperature is safe to call concurrently with DeltaTemperature, so a
// background weather refresh can update it while the regulation loop ticks.
func (heatLoss *HeatLossSimulator) SetOutdoorTemperature(t float64) {
	heatLoss.mu.Lock()
	heatLoss.outdoorTemp = t
	heatLoss.mu.Unlock()
}

func (heatLoss *HeatLossSimulator) OutdoorTemperature() float64 {
	heatLoss.mu.RLock()
	defer heatLoss.mu.RUnlock()
	return heatLoss.outdoorTemp
}

func (heatLoss *HeatLossSimulator) DeltaTemperature(indoorTemperature float64, dt time.Duration) float64 {
	heatLoss.mu.RLock()
	outdoor, coefficient := heatLoss.outdoorTemp, heatLoss.coefficient
	heatLoss.mu.RUnlock()

	diff := outdoor - indoorTemperature
	return coefficient * diff * dt.Seconds()
}
