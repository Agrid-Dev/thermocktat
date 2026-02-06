package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"time"

	"github.com/Agrid-Dev/thermocktat/internal/thermostat"
)

type SetpointCommand struct {
	IterationNumber int
	Value           float64
}

func SimulateThermostat(iterations int, filename string, setpointCommands []SetpointCommand) error {
	// Initialize thermostat with some parameters
	initial := thermostat.Snapshot{
		Enabled:                true,
		TemperatureSetpoint:    20.0,
		TemperatureSetpointMin: 15.0,
		TemperatureSetpointMax: 30.0,
		Mode:                   thermostat.ModeHeat,
		FanSpeed:               thermostat.FanAuto,
		AmbientTemperature:     20.0,
	}

	pidParams := thermostat.PIDRegulatorParams{
		Kp:                   0.00001,
		Ki:                   0.00001,
		Kd:                   0.01,
		ModeChangeHysteresis: 2.0,
		TargetHysteresis:     1.0,
	}
	heatLoss := thermostat.HeatLossSimulatorParams{
		Coefficient:        1.e-4,
		OutdoorTemperature: 10,
	}

	thermostat, err := thermostat.New(initial, pidParams, heatLoss)
	if err != nil {
		return fmt.Errorf("failed to create thermostat: %v", err)
	}

	// Create CSV file
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %v", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write CSV header
	if err := writer.Write([]string{"Iteration", "Ambient", "Setpoint", "ModeChangeLow", "ModeChangeHigh", "TargetLow", "TargetHigh"}); err != nil {
		return fmt.Errorf("failed to write CSV header: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run simulation
	for i := range iterations {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Check if we need to update the setpoint
			for _, cmd := range setpointCommands {
				if cmd.IterationNumber == i+1 {
					if err := thermostat.SetSetpoint(cmd.Value); err != nil {
						return fmt.Errorf("failed to update setpoint: %v", err)
					}
					break
				}
			}

			// Get current state
			snapshot := thermostat.Get()

			// Write to CSV
			if err := writer.Write([]string{
				fmt.Sprintf("%d", i+1),
				fmt.Sprintf("%.2f", snapshot.AmbientTemperature),
				fmt.Sprintf("%.2f", snapshot.TemperatureSetpoint),
				fmt.Sprintf("%.2f", snapshot.TemperatureSetpoint-pidParams.ModeChangeHysteresis),
				fmt.Sprintf("%.2f", snapshot.TemperatureSetpoint+pidParams.ModeChangeHysteresis),
				fmt.Sprintf("%.2f", snapshot.TemperatureSetpoint-pidParams.TargetHysteresis),
				fmt.Sprintf("%.2f", snapshot.TemperatureSetpoint+pidParams.TargetHysteresis),
			}); err != nil {
				return fmt.Errorf("failed to write CSV record: %v", err)
			}

			// Update ambient temperature
			thermostat.UpdateAmbient(time.Second)

		}
	}

	return nil
}

func main() {
	commands := []SetpointCommand{
		{
			IterationNumber: 200,
			Value:           22.0,
		},
	}
	SimulateThermostat(1000, "thermocktat.csv", commands)
}
