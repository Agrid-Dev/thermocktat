package thermostat

import (
	"math"
	"testing"
	"time"
)

func testValidatePIDRegulatorParams(t *testing.T) {
	paramsOk := PIDRegulatorParams{
		Kp:                   0.1,
		Ki:                   0.01,
		Kd:                   0.05,
		ModeChangeHysteresis: 1.0,
		TargetHysteresis:     0.5,
	}
	err := paramsOk.Validate()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	paramsInvalid := PIDRegulatorParams{
		Kp:                   0.1,
		Ki:                   0.01,
		Kd:                   0.05,
		ModeChangeHysteresis: 1.0,
		TargetHysteresis:     2.0,
	}
	if paramsInvalid.Validate() != ErrInvalidRegulatorHysteresis {
		t.Errorf("Expected error, got %v", err)
	}
}

func almostEqual(a, b, tolerance float64) bool {
	return math.Abs(a-b) <= tolerance
}

func TestGetTarget(t *testing.T) {
	params := PIDRegulatorParams{
		Kp:                   0.1,
		Ki:                   0.01,
		Kd:                   0.05,
		ModeChangeHysteresis: 1.0,
		TargetHysteresis:     0.5,
	}
	tests := []struct {
		name      string
		setpoint  float64
		ambient   float64
		mode      Mode
		isHeating bool
		isCooling bool
		want      float64
	}{
		{"Heat is Heating", 20.0, 21.0, ModeHeat, true, false, 20.5},
		{"Heat not is Heating", 20.0, 21.0, ModeHeat, false, false, 20.0},
		{"Cool is Cooling", 20.0, 19.0, ModeCool, false, true, 19.5},
		{"Cool not is Cooling", 20.0, 19.0, ModeCool, false, false, 20.0},
		{"Auto is heating", 20.0, 21.0, ModeAuto, true, false, 20.5},
		{"Auto is cooling", 20.0, 19.0, ModeAuto, false, true, 19.5},
		{"Auto not heating or cooling", 20.0, 20.0, ModeAuto, false, false, 20.0},
		{"Fan", 20.0, 21.0, ModeFan, false, false, 20.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regulator := NewPIDRegulator(params)
			regulator.isHeating = tt.isHeating
			regulator.isCooling = tt.isCooling
			got := regulator.GetTarget(tt.setpoint, tt.ambient, tt.mode)
			if got != tt.want {
				t.Errorf("GetTarget() = %v, want %v", got, tt.want)
			}
		})
	}
}

func assertRegulatorState(t testing.TB, regulator *PIDRegulator, wantHeating bool, wantCooling bool) {
	t.Helper()
	if regulator.isHeating != wantHeating {
		t.Errorf("isHeating = %v, want %v", regulator.isHeating, wantHeating)
	}
	if regulator.isCooling != wantCooling {
		t.Errorf("isCooling = %v, want %v", regulator.isCooling, wantCooling)
	}
}

func TestPIDActivate(t *testing.T) {
	params := PIDRegulatorParams{
		Kp:                   0.1,
		Ki:                   0.01,
		Kd:                   0.05,
		TargetHysteresis:     1.0,
		ModeChangeHysteresis: 2.0,
	}
	tests := []struct {
		name        string
		setpoint    float64
		ambient     float64
		mode        Mode
		wantHeating bool
		wantCooling bool
	}{
		{"Neutral when ambiant = setpoint", 20.0, 20.0, ModeHeat, false, false},
		{"Neutral above setpoint - heat", 20.0, 20.5, ModeHeat, false, false},
		{"Neutral within hysteresis - heat", 20.0, 19.5, ModeHeat, false, false},
		{"Neutral below setpoint - cool", 20.0, 19.5, ModeCool, false, false},
		{"Neutral within hysteresis - cool", 20.0, 20.5, ModeCool, false, false},
		{"Activate heating", 22.0, 20.0, ModeHeat, true, false},
		{"Activate cooling", 20.0, 22.0, ModeCool, false, true},
		{"Activate heating (auto)", 23.0, 20.0, ModeAuto, true, false},
		{"Activate cooling (auto)", 20.0, 23.0, ModeAuto, false, true},
		{"Activate nothing (fan)", 22.0, 20.0, ModeFan, false, false},
		{"Activate nothing (fan)", 20.0, 22.0, ModeFan, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regulator := NewPIDRegulator(params)
			regulator.Activate(tt.setpoint, tt.ambient, tt.mode)
			assertRegulatorState(t, regulator, tt.wantHeating, tt.wantCooling)
		})
	}
}

func TestPIDDeActivate(t *testing.T) {
	params := PIDRegulatorParams{
		Kp:                   0.1,
		Ki:                   0.01,
		Kd:                   0.05,
		ModeChangeHysteresis: 2.0,
		TargetHysteresis:     1,
	}
	tests := []struct {
		name             string
		setpoint         float64
		ambient          float64
		mode             Mode
		initiallyHeating bool
		wantHeating      bool
		initiallyCooling bool
		wantCooling      bool
	}{
		{"Keep heating while setpoint not reached", 22.0, 20.0, ModeHeat, true, true, false, false},
		{"Keep heating while setpoint + hysteresis not reached", 22.0, 22.5, ModeHeat, true, true, false, false},
		{"Keep cooling while setpoint not reached", 20.0, 22.0, ModeCool, false, false, true, true},
		{"Keep cooling while setpoint - hysteresis not reached", 20.0, 19.5, ModeCool, false, false, true, true},
		{"Keep heating while setpoint not reached (auto/heating)", 22.0, 20.0, ModeAuto, true, true, false, false},
		{"Keep heating while setpoint + hysteresis not reached (auto/heating)", 22.0, 22.5, ModeAuto, true, true, false, false},
		{"Keep cooling while setpoint  not reached (auto/cooling)", 20.0, 22.0, ModeAuto, false, false, true, true},
		{"Keep cooling while setpoint - hysteresis not reached (auto/cooling)", 20.0, 19.5, ModeAuto, false, false, true, true},
		{"Stop heating when setpoint + hysteresis reached", 22.0, 23.5, ModeHeat, true, false, false, false},
		{"Stop cooling when setpoint - hysteresis reached", 20.0, 18.5, ModeCool, false, false, true, false},
		{"Stop heating when setpoint + hysteresis reached (auto/heating)", 22.0, 23.5, ModeAuto, true, false, false, false},
		{"Stop cooling when setpoint - hysteresis reached (auto/cooling)", 20.0, 18.5, ModeAuto, false, false, true, false},
		{"Stop heating (fan)", 22.0, 20.0, ModeFan, true, false, false, false},
		{"Stop cooling (fan)", 20.0, 22.0, ModeFan, false, false, true, false},
		{"Start heating when temperature below setpoint - hysteresis", 22.0, 20.0, ModeHeat, false, true, false, false},
		{"Start cooling when temperature above setpoint + hysteresis", 20.0, 22.0, ModeCool, false, false, false, true},
		{"Don't start heating when temperature within setpoint - hysteresis", 22.0, 21.5, ModeHeat, false, false, false, false},
		{"Don't start cooling when temperature within setpoint + hysteresis", 20.0, 20.5, ModeCool, false, false, false, false},
		{"Change mode if temperature difference above ModeChange hysteresis (auto/heat->cool)", 20.0, 25.0, ModeAuto, true, false, false, true},
		{"Change mode if temperature difference above ModeChange hysteresis (auto/cool->heat)", 25.0, 20.0, ModeAuto, false, true, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regulator := NewPIDRegulator(params)
			regulator.isHeating = tt.initiallyHeating
			regulator.isCooling = tt.initiallyCooling
			regulator.Activate(tt.setpoint, tt.ambient, tt.mode)
			assertRegulatorState(t, regulator, tt.wantHeating, tt.wantCooling)
		})
	}
}

func TestPIDRegulatorStart(t *testing.T) {
	params := PIDRegulatorParams{
		Kp:                   0.1,
		Ki:                   0.01,
		Kd:                   0.05,
		ModeChangeHysteresis: 1.0,
		TargetHysteresis:     0.5,
	}

	approx := 0.05
	exact := 0.0

	tests := []struct {
		name      string
		setpoint  float64
		ambient   float64
		mode      Mode
		want      float64
		tolerance float64
	}{
		{"HeatModeReg", 25.0, 20.0, ModeHeat, 20.88, approx}, // outside hysteresis : start regulating
		{"CoolModeReg", 20.0, 25.0, ModeCool, 24.12, approx},
		{"FanModeReg", 25.0, 20.0, ModeFan, 20.0, approx},
		{"FanModeReg", 20.0, 25.0, ModeFan, 25.0, approx},
		{"AutoModeRegHeat", 25.0, 20.0, ModeAuto, 20.88, approx},
		{"AutoModeRegCool", 20.0, 25.0, ModeAuto, 24.12, approx},
		{"HeatModeRegWithinHysteresisUp", 20.0, 20.5, ModeHeat, 20.5, exact}, // within hysteresis : no regulation
		{"HeatModeRegWithinHysteresisDown", 20.0, 19.5, ModeHeat, 19.5, exact},
		{"CoolModeRegWithinHysteresisUp", 20.0, 20.5, ModeCool, 20.5, exact},
		{"CoolModeRegWithinHysteresisDown", 20.0, 19.5, ModeCool, 19.5, exact},
		{"AutoModeRegWithinHysteresisUp", 20.0, 20.4, ModeAuto, 20.4, exact},
		{"AutoModeRegWithinHysteresisDown", 20.0, 19.6, ModeAuto, 19.6, exact},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			regulator := NewPIDRegulator(params)
			got := regulator.Update(tt.setpoint, tt.ambient, tt.mode, time.Second)
			if !almostEqual(got, tt.want, tt.tolerance) {
				t.Errorf("PIDRegulator.Update() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPidRegulatorHeating(t *testing.T) {
	params := PIDRegulatorParams{
		Kp:                   0.1,
		Ki:                   0.01,
		Kd:                   0.05,
		ModeChangeHysteresis: 1.0,
		TargetHysteresis:     0.5,
	}
	regulator := NewPIDRegulator(params)
	if regulator.isHeating || regulator.isCooling {
		t.Errorf("Regulator should not be heating or cooling initially")
	}
	setpoint := 20.0
	var ambient = 18.0
	iterations := 0
	maxIterations := 1000

	for ambient < setpoint+params.TargetHysteresis && iterations < maxIterations {
		newAmbient := regulator.Update(setpoint, ambient, ModeHeat, time.Second)
		if newAmbient < ambient {
			t.Errorf("Ambient temperature should not decrease when heating, have %f < %f (iteration %d)", newAmbient, ambient, iterations)
			break
		}
		if !regulator.isHeating {
			t.Errorf("Regulator should be heating")
			break
		}
		if regulator.isCooling {
			t.Errorf("Regulator should not be cooling")
			break
		}
		ambient = newAmbient
		iterations++
	}
	if iterations == maxIterations {
		t.Errorf("Regulator did not reach the desired state within %d iterations", maxIterations)
	}
	regulator.Activate(setpoint, ambient, ModeHeat)
	if regulator.isHeating {
		t.Errorf("Regulator should be done heating after reaching setpoint + targetHysteresis")
	}

}

func TestThermostatUpdateAmbientTemperature(t *testing.T) {
	initial := Snapshot{
		Enabled:                true,
		TemperatureSetpoint:    22.5,
		Mode:                   ModeHeat,
		AmbientTemperature:     20.0,
		TemperatureSetpointMin: 18.0,
		TemperatureSetpointMax: 27.0,
		FanSpeed:               FanAuto,
	}
	params := PIDRegulatorParams{
		Kp:                   0.1,
		Ki:                   0.01,
		Kd:                   0.05,
		ModeChangeHysteresis: 1.0,
		TargetHysteresis:     0.5,
	}
	thermostat, err := New(initial, params)
	if err != nil {
		t.Fatalf("Failed to create thermostat: %v", err)
	}

	thermostat.UpdateAmbientTemperature(time.Second)
	got := thermostat.Get().AmbientTemperature
	want := 20.4
	tolerance := 0.1

	if !almostEqual(got, want, tolerance) {
		t.Errorf("Thermostat.UpdateAmbientTemperature() = %v, want %v", got, want)
	}
}
