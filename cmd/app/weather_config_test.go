package app

import (
	"context"
	"testing"

	"github.com/Agrid-Dev/thermocktat/internal/weather"
)

func TestEnvKeyTransform_WeatherProvider(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"WEATHER_PROVIDER_TYPE", "weather_provider.type"},
		{"WEATHER_PROVIDER_REFRESH_INTERVAL", "weather_provider.refresh_interval"},
		{"WEATHER_PROVIDER_OPEN_METEO_LATITUDE", "weather_provider.open_meteo.latitude"},
		{"WEATHER_PROVIDER_OPEN_METEO_LONGITUDE", "weather_provider.open_meteo.longitude"},
		{"WEATHER_PROVIDER_STATIC_OUTDOOR_TEMPERATURE", "weather_provider.static.outdoor_temperature"},
		{"WEATHER_PROVIDER", "weather_provider"}, // not enough parts -> passthrough
	}

	for _, tt := range tests {
		got := envKeyTransform(tt.in)
		if got != tt.want {
			t.Fatalf("envKeyTransform(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestWeatherProvider_DefaultsToStaticFromHeatLoss(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	p, err := cfg.WeatherProvider(nil)
	if err != nil {
		t.Fatalf("WeatherProvider: %v", err)
	}
	if _, ok := p.(*weather.Static); !ok {
		t.Fatalf("default provider = %T, want *weather.Static", p)
	}

	got, err := p.OutdoorTemperature(context.Background())
	if err != nil {
		t.Fatalf("OutdoorTemperature: %v", err)
	}
	if got != cfg.HeatLoss.OutdoorTemperature {
		t.Fatalf("static temp = %v, want heat_loss default %v", got, cfg.HeatLoss.OutdoorTemperature)
	}
}

func TestWeatherProvider_StaticOverrideWins(t *testing.T) {
	override := 27.5
	cfg := Config{
		HeatLoss: HeatLossConfig{OutdoorTemperature: 10},
		Weather: WeatherProviderConfig{
			Type:   "static",
			Static: StaticWeatherConfig{OutdoorTemperature: &override},
		},
	}

	p, err := cfg.WeatherProvider(nil)
	if err != nil {
		t.Fatalf("WeatherProvider: %v", err)
	}
	got, _ := p.OutdoorTemperature(context.Background())
	if got != override {
		t.Fatalf("static temp = %v, want override %v", got, override)
	}
}

func TestWeatherProvider_OpenMeteoSelected(t *testing.T) {
	cfg := Config{
		Weather: WeatherProviderConfig{
			Type:      "open-meteo",
			OpenMeteo: OpenMeteoWeatherConfig{Latitude: 48.8566, Longitude: 2.3522},
		},
	}

	p, err := cfg.WeatherProvider(nil)
	if err != nil {
		t.Fatalf("WeatherProvider: %v", err)
	}
	if _, ok := p.(*weather.OpenMeteo); !ok {
		t.Fatalf("provider = %T, want *weather.OpenMeteo", p)
	}
}

func TestWeatherProvider_InvalidTypeRejected(t *testing.T) {
	cfg := Config{Weather: WeatherProviderConfig{Type: "nope"}}
	if _, err := cfg.WeatherProvider(nil); err == nil {
		t.Fatal("expected error for invalid weather_provider.type")
	}
}

func TestLoadConfig_OpenMeteoFromEnv(t *testing.T) {
	t.Setenv("TMK_WEATHER_PROVIDER_TYPE", "open-meteo")
	t.Setenv("TMK_WEATHER_PROVIDER_OPEN_METEO_LATITUDE", "51.5074")
	t.Setenv("TMK_WEATHER_PROVIDER_OPEN_METEO_LONGITUDE", "-0.1278")

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Weather.Type != "open-meteo" {
		t.Fatalf("type = %q, want open-meteo", cfg.Weather.Type)
	}
	if cfg.Weather.OpenMeteo.Latitude != 51.5074 || cfg.Weather.OpenMeteo.Longitude != -0.1278 {
		t.Fatalf("coords = (%v, %v), want (51.5074, -0.1278)", cfg.Weather.OpenMeteo.Latitude, cfg.Weather.OpenMeteo.Longitude)
	}
}

func TestLoadConfig_InvalidLatitudeRejected(t *testing.T) {
	t.Setenv("TMK_WEATHER_PROVIDER_TYPE", "open-meteo")
	t.Setenv("TMK_WEATHER_PROVIDER_OPEN_METEO_LATITUDE", "120")

	if _, err := LoadConfig(""); err == nil {
		t.Fatal("expected error for out-of-range latitude")
	}
}
