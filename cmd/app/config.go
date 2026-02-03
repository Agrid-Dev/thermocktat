package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	kjson "github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"

	"github.com/Agrid-Dev/thermocktat/internal/thermostat"
)

type Config struct {
	DeviceID string `koanf:"device_id" json:"device_id" yaml:"device_id"`

	// Convenience keys:
	// If Controller is set, only that controller is enabled (http|mqtt|modbus).
	// If Addr is set alongside Controller, it is copied to the chosen controller's addr.
	Controller string `koanf:"controller" json:"controller" yaml:"controller"`
	Addr       string `koanf:"addr" json:"addr" yaml:"addr"`

	Controllers struct {
		HTTP   HTTPConfig   `koanf:"http" json:"http" yaml:"http"`
		MQTT   MQTTConfig   `koanf:"mqtt" json:"mqtt" yaml:"mqtt"`
		MODBUS Modbusconfig `koanf:"modbus" json:"modbus" yaml:"modbus"`
	} `koanf:"controllers" json:"controllers" yaml:"controllers"`

	Thermostat ThermostatConfig `koanf:"thermostat" json:"thermostat" yaml:"thermostat"`
	Regulator  RegulatorConfig  `koanf:"regulator" json:"regulator" yaml:"regulator"`
}

type ThermostatConfig struct {
	Enabled            *bool    `koanf:"enabled" json:"enabled" yaml:"enabled"`
	AmbientTemperature *float64 `koanf:"ambient_temperature" json:"ambient_temperature" yaml:"ambient_temperature"`

	Setpoint    *float64 `koanf:"temperature_setpoint" json:"temperature_setpoint" yaml:"temperature_setpoint"`
	SetpointMin *float64 `koanf:"temperature_setpoint_min" json:"temperature_setpoint_min" yaml:"temperature_setpoint_min"`
	SetpointMax *float64 `koanf:"temperature_setpoint_max" json:"temperature_setpoint_max" yaml:"temperature_setpoint_max"`

	Mode     *string `koanf:"mode" json:"mode" yaml:"mode"`                // "heat" | "cool" | "fan" | "auto"
	FanSpeed *string `koanf:"fan_speed" json:"fan_speed" yaml:"fan_speed"` // "auto" | "low" | "medium" | "high"
}

type RegulatorConfig struct {
	Enabled           bool          `koanf:"enabled" json:"enabled" yaml:"enabled"`
	Interval          time.Duration `koanf:"interval" json:"interval" yaml:"interval"`
	Kp                float64       `koanf:"p" json:"p" yaml:"p"`
	Ki                float64       `koanf:"i" json:"i" yaml:"i"`
	Kd                float64       `koanf:"d" json:"d" yaml:"d"`
	TriggerHysteresis float64       `koanf:"trigger_hysteresis" json:"trigger_hysteresis" yaml:"trigger_hysteresis"`
	TargetHysteresis  float64       `koanf:"target_hysteresis" json:"target_hysteresis" yaml:"target_hysteresis"`
}

type HTTPConfig struct {
	Enabled bool   `koanf:"enabled" json:"enabled" yaml:"enabled"`
	Addr    string `koanf:"addr" json:"addr" yaml:"addr"`
}

type MQTTConfig struct {
	Enabled         bool          `koanf:"enabled" json:"enabled" yaml:"enabled"`
	Addr            string        `koanf:"addr" json:"addr" yaml:"addr"`
	ClientID        string        `koanf:"client_id" json:"client_id" yaml:"client_id"`
	BaseTopic       string        `koanf:"base_topic" json:"base_topic" yaml:"base_topic"`
	QoS             byte          `koanf:"qos" json:"qos" yaml:"qos"`
	RetainSnapshot  bool          `koanf:"retain_snapshot" json:"retain_snapshot" yaml:"retain_snapshot"`
	PublishMode     string        `koanf:"publish_mode" json:"publish_mode" yaml:"publish_mode"`
	PublishInterval time.Duration `koanf:"publish_interval" json:"publish_interval" yaml:"publish_interval"`
	Username        string        `koanf:"username" json:"username" yaml:"username"`
	Password        string        `koanf:"password" json:"password" yaml:"password"`
}

type Modbusconfig struct {
	Enabled      bool          `koanf:"enabled" json:"enabled" yaml:"enabled"`
	Addr         string        `koanf:"addr" json:"addr" yaml:"addr"`
	UnitID       byte          `koanf:"unit_id" json:"unit_id" yaml:"unit_id"`
	SyncInterval time.Duration `koanf:"sync_interval" json:"sync_interval" yaml:"sync_interval"`
}

func DefaultConfig() Config {
	var cfg Config

	cfg.DeviceID = "default"

	cfg.Controllers.HTTP.Addr = ":8080"
	cfg.Controllers.MQTT.PublishInterval = 1 * time.Second
	cfg.Controllers.MODBUS.UnitID = 1

	cfg.Regulator.Kp = 0.1
	cfg.Regulator.Ki = 0.01
	cfg.Regulator.Kd = 0.05
	cfg.Regulator.Interval = 1 * time.Second

	return cfg
}

func LoadConfig(path string) (Config, error) {
	k := koanf.New(".")

	// 1) Defaults
	if err := k.Load(structs.Provider(DefaultConfig(), "koanf"), nil); err != nil {
		return Config{}, fmt.Errorf("load defaults: %w", err)
	}

	// 2) Optional file layer
	if path != "" {
		if _, err := os.Stat(path); err != nil {
			if !os.IsNotExist(err) {
				return Config{}, fmt.Errorf("stat config file: %w", err)
			}
			// missing file is OK → defaults + env only
		} else {
			ext := strings.ToLower(filepath.Ext(path))
			switch ext {
			case ".yaml", ".yml":
				if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
					return Config{}, fmt.Errorf("load yaml config: %w", err)
				}
			case ".json":
				if err := k.Load(file.Provider(path), kjson.Parser()); err != nil {
					return Config{}, fmt.Errorf("load json config: %w", err)
				}
			default:
				return Config{}, fmt.Errorf("unsupported config extension %q", ext)
			}
		}
	}

	// 3) Env layer (TMK_*), overrides file + defaults
	if err := k.Load(
		env.Provider(".", env.Opt{
			Prefix: "TMK_",
			TransformFunc: func(k, v string) (string, any) {
				k = strings.TrimPrefix(k, "TMK_")
				key := envKeyTransform(k)
				if key == "" {
					return "", nil // ignore
				}
				return key, v
			},
		}),
		nil,
	); err != nil {
		return Config{}, fmt.Errorf("load env: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	normalize(&cfg)
	if err := validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// envKeyTransform maps TMK_* env vars to koanf keys.
//
// Rules:
// - TMK_DEVICE_ID          -> device_id
// - TMK_CONTROLLER         -> controller
// - TMK_ADDR               -> addr
// - TMK_CONTROLLERS_HTTP_ADDR              -> controllers.http.addr
// - TMK_CONTROLLERS_MQTT_PUBLISH_INTERVAL  -> controllers.mqtt.publish_interval
// - TMK_THERMOSTAT_TEMPERATURE_SETPOINT    -> thermostat.temperature_setpoint
// - TMK_REGULATOR_TRIGGER_HYSTERESIS       -> regulator.trigger_hysteresis
func envKeyTransform(k string) string {
	// k is the env var name without the prefix "TMK_"
	key := strings.ToLower(strings.TrimSpace(k))
	if key == "" {
		return ""
	}

	parts := strings.Split(key, "_")
	if len(parts) == 0 {
		return key
	}

	switch parts[0] {
	case "controllers":
		// controllers_<ctrl>_<field...> -> controllers.<ctrl>.<field_with_underscores>
		if len(parts) < 3 {
			return key
		}
		ctrl := parts[1]
		field := strings.Join(parts[2:], "_")
		return "controllers." + ctrl + "." + field

	case "thermostat":
		// thermostat_<field...> -> thermostat.<field_with_underscores>
		if len(parts) < 2 {
			return key
		}
		field := strings.Join(parts[1:], "_")
		return "thermostat." + field

	case "regulator":
		// regulator_<field...> -> regulator.<field_with_underscores>
		if len(parts) < 2 {
			return key
		}
		field := strings.Join(parts[1:], "_")
		return "regulator." + field

	default:
		// top-level keys keep underscores (device_id, controller, addr, etc.)
		return key
	}
}

func normalize(cfg *Config) {
	// Convenience: controller + addr -> enable and disable all others
	if cfg.Controller != "" {
		c := strings.ToLower(strings.TrimSpace(cfg.Controller))
		cfg.Controllers.HTTP.Enabled = false
		cfg.Controllers.MQTT.Enabled = false
		cfg.Controllers.MODBUS.Enabled = false

		switch c {
		case "http":
			cfg.Controllers.HTTP.Enabled = true
			if cfg.Addr != "" {
				cfg.Controllers.HTTP.Addr = cfg.Addr
			}
		case "mqtt":
			cfg.Controllers.MQTT.Enabled = true
			if cfg.Addr != "" {
				cfg.Controllers.MQTT.Addr = cfg.Addr
			}
		case "modbus":
			cfg.Controllers.MODBUS.Enabled = true
			if cfg.Addr != "" {
				cfg.Controllers.MODBUS.Addr = cfg.Addr
			}
		}
	}

	// Common container fallback: PORT (only affects HTTP if enabled and addr not explicitly set)
	if cfg.Controllers.HTTP.Enabled && strings.TrimSpace(cfg.Controllers.HTTP.Addr) == "" {
		if p := strings.TrimSpace(os.Getenv("PORT")); p != "" {
			cfg.Controllers.HTTP.Addr = ":" + p
		}
	}
}

func validate(cfg Config) error {
	if cfg.Controller != "" {
		switch strings.ToLower(strings.TrimSpace(cfg.Controller)) {
		case "http", "mqtt", "modbus":
		default:
			return fmt.Errorf("invalid controller %q (expected http|mqtt|modbus)", cfg.Controller)
		}
	}

	if cfg.Controllers.HTTP.Enabled && strings.TrimSpace(cfg.Controllers.HTTP.Addr) == "" {
		return errors.New("http controller enabled but controllers.http.addr is empty")
	}
	if cfg.Controllers.MQTT.Enabled && strings.TrimSpace(cfg.Controllers.MQTT.Addr) == "" {
		return errors.New("mqtt controller enabled but controllers.mqtt.addr is empty")
	}
	if cfg.Controllers.MODBUS.Enabled && strings.TrimSpace(cfg.Controllers.MODBUS.Addr) == "" {
		return errors.New("modbus controller enabled but controllers.modbus.addr is empty")
	}

	if cfg.Regulator.Interval < 0 {
		return errors.New("regulator.interval must be >= 0")
	}
	if cfg.Controllers.MQTT.PublishInterval < 0 {
		return errors.New("controllers.mqtt.publish_interval must be >= 0")
	}
	if cfg.Controllers.MODBUS.SyncInterval < 0 {
		return errors.New("controllers.modbus.sync_interval must be >= 0")
	}

	return nil
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

func (c Config) RegulatorParams() (thermostat.PIDRegulatorParams, error) {
	params := thermostat.PIDRegulatorParams{
		Kp:                c.Regulator.Kp,
		Ki:                c.Regulator.Ki,
		Kd:                c.Regulator.Kd,
		TriggerHysteresis: c.Regulator.TriggerHysteresis,
		TargetHysteresis:  c.Regulator.TargetHysteresis,
	}
	if err := params.Validate(); err != nil {
		return thermostat.PIDRegulatorParams{}, err
	}
	return params, nil
}
