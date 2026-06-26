package thermostat_test

import (
	"context"
	"testing"
	"time"

	"github.com/Agrid-Dev/thermocktat/internal/testutil"
	"github.com/Agrid-Dev/thermocktat/internal/thermostat"
)

func newDisabledThermostat(t *testing.T, ambient, outdoor, coefficient float64) *thermostat.Thermostat {
	t.Helper()
	th, err := thermostat.New(
		thermostat.Snapshot{
			Enabled:                false,
			TemperatureSetpoint:    22,
			TemperatureSetpointMin: 16,
			TemperatureSetpointMax: 28,
			Mode:                   thermostat.ModeAuto,
			FanSpeed:               thermostat.FanAuto,
			AmbientTemperature:     ambient,
		},
		thermostat.PIDRegulatorParams{Kp: 0.001, Ki: 0.001, Kd: 0.01, TargetHysteresis: 1, ModeChangeHysteresis: 2},
		thermostat.HeatLossSimulatorParams{Coefficient: coefficient, OutdoorTemperature: outdoor},
		nil,
	)
	if err != nil {
		t.Fatalf("new thermostat: %v", err)
	}
	return th
}

// ambientDrift returns the change in ambient temperature over one second of
// heat-loss simulation with regulation disabled. Its sign reveals whether the
// outdoor temperature currently pulls ambient up or down.
func ambientDrift(th *thermostat.Thermostat) float64 {
	before := th.Get().AmbientTemperature
	th.UpdateAmbient(time.Second)
	return th.Get().AmbientTemperature - before
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition not met within deadline")
}

func TestRunWeatherRefreshAppliesProviderValue(t *testing.T) {
	// Outdoor starts equal to ambient -> no drift until the provider raises it.
	th := newDisabledThermostat(t, 20, 20, 0.5)
	provider := testutil.NewFakeWeatherProvider(30)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = th.RunWeatherRefresh(ctx, provider, 10*time.Millisecond) }()

	waitFor(t, func() bool { return provider.CallCount() >= 1 })
	cancel()

	if drift := ambientDrift(th); drift <= 0 {
		t.Fatalf("expected warming drift after outdoor rose to 30, got %v", drift)
	}
}

func TestRunWeatherRefreshFollowsSequence(t *testing.T) {
	// Outdoor below ambient -> cooling drift; the provider then reports a higher
	// value, which must flip the drift to warming.
	th := newDisabledThermostat(t, 25, 10, 0.5)
	provider := &testutil.FakeWeatherProvider{Temps: []float64{10, 40}}

	if drift := ambientDrift(th); drift >= 0 {
		t.Fatalf("expected initial cooling drift toward 10, got %v", drift)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = th.RunWeatherRefresh(ctx, provider, 5*time.Millisecond) }()

	waitFor(t, func() bool { return provider.CallCount() >= 2 })
	cancel()

	if drift := ambientDrift(th); drift <= 0 {
		t.Fatalf("expected warming drift after outdoor rose to 40, got %v", drift)
	}
}

func TestRunWeatherRefreshNoOpWhenDisabled(t *testing.T) {
	th := newDisabledThermostat(t, 20, 20, 0.5)
	provider := testutil.NewFakeWeatherProvider(30)

	// nil provider -> returns immediately, no refresh.
	if err := th.RunWeatherRefresh(context.Background(), nil, time.Hour); err != nil {
		t.Fatalf("nil provider should return nil, got %v", err)
	}
	// zero interval -> disabled, returns immediately without calling the provider.
	if err := th.RunWeatherRefresh(context.Background(), provider, 0); err != nil {
		t.Fatalf("zero interval should return nil, got %v", err)
	}
	if provider.CallCount() != 0 {
		t.Fatalf("provider should not be called when disabled, got %d calls", provider.CallCount())
	}
}
