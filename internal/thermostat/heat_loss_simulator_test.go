package thermostat

import (
	"testing"
	"time"
)

func TestValidateParams(t *testing.T) {
	tests := []struct {
		name   string
		params HeatLossSimulatorParams
		want   error
	}{
		{
			name: "Valid params",
			params: HeatLossSimulatorParams{
				OutdoorTemperature: 10,
				Coefficient:        5,
			},
			want: nil,
		},
		{
			name: "Invalid params with negative coefficient",
			params: HeatLossSimulatorParams{
				OutdoorTemperature: 10,
				Coefficient:        -5,
			},
			want: ErrNegativeHeatLossCoefficient,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.params.Validate()
			if got != tt.want {
				t.Errorf("Got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHeatLossDeltaTemperature(t *testing.T) {
	tests := []struct {
		name        string
		outdoorTemp float64
		indoorTemp  float64
		want        func(float64) bool
	}{
		{
			name:        "Indoor temperature decreases if outdoor temperature is less",
			outdoorTemp: 5,
			indoorTemp:  20,
			want:        func(result float64) bool { return result < 0 },
		},
		{
			name:        "Indoor temperature increases if outdoor temperature is more",
			outdoorTemp: 30,
			indoorTemp:  20,
			want:        func(result float64) bool { return result > 0 },
		},
		{
			name:        "Indoor temperature is unchanged if equal to outdoor temperature",
			outdoorTemp: 20,
			indoorTemp:  20,
			want:        func(result float64) bool { return result == 0 },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := HeatLossSimulatorParams{
				OutdoorTemperature: tt.outdoorTemp,
				Coefficient:        5,
			}
			regulator, _ := NewHeatLossSimulator(params)
			result := regulator.DeltaTemperature(tt.indoorTemp, time.Second)
			if !tt.want(result) {
				t.Errorf("Test %q failed: got %v, initial %v", tt.name, result, tt.indoorTemp)
			}
		})
	}
}
