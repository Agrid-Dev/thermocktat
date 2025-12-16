package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Agrid-Dev/thermocktat/internal/thermostat"
)

type Config struct {
	HTTP struct {
		Addr string `json:"addr" yaml:"addr"`
	} `json:"http" yaml:"http"`

	Thermostat ThermostatConfig `json:"thermostat" yaml:"thermostat"`
}

type ThermostatConfig struct {
	Enabled            *bool    `json:"enabled" yaml:"enabled"`
	AmbientTemperature *float64 `json:"ambient_temperature" yaml:"ambient_temperature"`

	Setpoint    *float64 `json:"temperature_setpoint" yaml:"temperature_setpoint"`
	SetpointMin *float64 `json:"temperature_setpoint_min" yaml:"temperature_setpoint_min"`
	SetpointMax *float64 `json:"temperature_setpoint_max" yaml:"temperature_setpoint_max"`

	Mode     *string `json:"mode" yaml:"mode"`           // "heat" | "cool" | "fan" | "auto"
	FanSpeed *string `json:"fan_speed" yaml:"fan_speed"` // "auto" | "low" | "medium" | "high"
}

func LoadConfig(path string) (Config, error) {
	var cfg Config

	if path == "" {
		applyDefaults(&cfg)
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Config file missing → use defaults
			applyDefaults(&cfg)
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("parse yaml: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("parse json: %w", err)
		}
	default:
		return cfg, fmt.Errorf("unsupported config extension %q", ext)
	}

	applyDefaults(&cfg)
	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.HTTP.Addr == "" {
		cfg.HTTP.Addr = ":8080"
	}
}

func (c Config) Snapshot() (thermostat.Snapshot, error) {
	// Defaults
	enabled := true
	ambient := 21.0
	sp := 22.0
	min := 16.0
	max := 28.0
	modeStr := "auto"
	fanStr := "auto"

	// Apply overrides if set
	if c.Thermostat.Enabled != nil {
		enabled = *c.Thermostat.Enabled
	}
	if c.Thermostat.AmbientTemperature != nil {
		ambient = *c.Thermostat.AmbientTemperature
	}
	if c.Thermostat.Setpoint != nil {
		sp = *c.Thermostat.Setpoint
	}
	if c.Thermostat.SetpointMin != nil {
		min = *c.Thermostat.SetpointMin
	}
	if c.Thermostat.SetpointMax != nil {
		max = *c.Thermostat.SetpointMax
	}
	if c.Thermostat.Mode != nil {
		modeStr = *c.Thermostat.Mode
	}
	if c.Thermostat.FanSpeed != nil {
		fanStr = *c.Thermostat.FanSpeed
	}

	mode, err := thermostat.ParseMode(modeStr)
	if err != nil {
		return thermostat.Snapshot{}, err
	}
	fan, err := thermostat.ParseFanSpeed(fanStr)
	if err != nil {
		return thermostat.Snapshot{}, err
	}

	return thermostat.Snapshot{
		Enabled:                enabled,
		TemperatureSetpoint:    sp,
		TemperatureSetpointMin: min,
		TemperatureSetpointMax: max,
		Mode:                   mode,
		FanSpeed:               fan,
		AmbientTemperature:     ambient,
	}, nil
}

func ApplyEnvOverrides(cfg *Config) {
	// Explicit addr prefered, else support PORT (common in containers).
	if v := os.Getenv("THERMOCKSTAT_HTTP_ADDR"); v != "" {
		cfg.HTTP.Addr = v
		return
	}
	if v := os.Getenv("PORT"); v != "" {
		// listen on all interfaces on that port
		cfg.HTTP.Addr = ":" + v
		return
	}
}
