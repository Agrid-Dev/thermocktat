package mqttctrl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Agrid-Dev/thermocktat/internal/ports"
	"github.com/Agrid-Dev/thermocktat/internal/thermostat"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	PublishOnChange string = "on_change"
	PublishInterval string = "interval"
)

type Config struct {
	// Identity
	DeviceID string

	// MQTT connection
	BrokerURL string
	ClientID  string

	// Topics
	BaseTopic string

	// Behavior
	QoS             byte
	RetainSnapshot  bool
	PublishInterval time.Duration
	// PublishMode controls when snapshots are published:
	// - "on_change": publish only when snapshot has changed (default)
	// - "interval":  publish every PublishInterval even if unchanged
	PublishMode string

	Username string
	Password string
}

type Controller struct {
	svc ports.ThermostatService
	cfg Config
	log *slog.Logger

	client mqtt.Client
}

func New(svc ports.ThermostatService, cfg Config, logger *slog.Logger) (*Controller, error) {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	if cfg.BrokerURL == "" {
		cfg.BrokerURL = "tcp://localhost:1883"
	}
	if cfg.DeviceID == "" {
		return nil, errors.New("mqtt: DeviceID is required")
	}
	if cfg.BaseTopic == "" {
		cfg.BaseTopic = "thermocktat/" + cfg.DeviceID
	}
	if cfg.ClientID == "" {
		cfg.ClientID = "thermocktat-" + cfg.DeviceID
	}
	if cfg.PublishInterval <= 0 {
		cfg.PublishInterval = 1 * time.Second
	}
	// Default publish mode is on_change
	if cfg.PublishMode == "" {
		cfg.PublishMode = PublishOnChange
	}
	if cfg.PublishMode != PublishOnChange && cfg.PublishMode != PublishInterval {
		return nil, fmt.Errorf("mqtt: invalid PublishMode %q", cfg.PublishMode)
	}
	if cfg.QoS > 1 {
		return nil, errors.New("mqtt: QoS must be 0 or 1")
	}
	return &Controller{
		svc: svc,
		cfg: cfg,
		log: logger,
	}, nil
}

func (c *Controller) Run(ctx context.Context) error {
	opts := mqtt.NewClientOptions().
		AddBroker(c.cfg.BrokerURL).
		SetClientID(c.cfg.ClientID).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(2 * time.Second)

	if c.cfg.Username != "" {
		opts.SetUsername(c.cfg.Username)
		opts.SetPassword(c.cfg.Password)
	}

	// Subscribe when connected/reconnected.
	opts.OnConnect = func(cl mqtt.Client) {
		c.log.Info("mqtt broker connected",
			"base_topic", c.cfg.BaseTopic,
			"publish_mode", c.cfg.PublishMode,
		)
		topicSet := c.topic("set/+")
		tokenSet := cl.Subscribe(topicSet, c.cfg.QoS, c.onMessage)
		tokenSet.Wait()

		topicGet := c.topic("get/+")
		tokenGet := cl.Subscribe(topicGet, c.cfg.QoS, c.onMessage)
		tokenGet.Wait()
	}

	c.client = mqtt.NewClient(opts)
	tok := c.client.Connect()
	tok.Wait()
	if err := tok.Error(); err != nil {
		return fmt.Errorf("mqtt connect: %w", err)
	}

	// Publish loop: publish snapshot on interval, and only when changed.
	ticker := time.NewTicker(c.cfg.PublishInterval)
	defer ticker.Stop()

	c.publishSnapshot()

	for {
		select {
		case <-ctx.Done():
			c.client.Disconnect(250)
			return ctx.Err()

		case <-ticker.C:
			if c.cfg.PublishMode == PublishInterval {
				c.publishSnapshot()
			}
		}
	}
}

func (c *Controller) publishSnapshot() {
	s := c.svc.Get()
	dto := snapshotDTO{
		Enabled:                s.Enabled,
		TemperatureSetpoint:    s.TemperatureSetpoint,
		TemperatureSetpointMin: s.TemperatureSetpointMin,
		TemperatureSetpointMax: s.TemperatureSetpointMax,
		Mode:                   s.Mode.String(),
		FanSpeed:               s.FanSpeed.String(),
		AmbientTemperature:     s.AmbientTemperature,
		FaultCode:              s.FaultCode,
		DeviceId:               c.cfg.DeviceID,
	}

	b, _ := json.Marshal(dto)
	c.client.Publish(c.topic("snapshot"), c.cfg.QoS, c.cfg.RetainSnapshot, b)
}

type snapshotDTO struct {
	Enabled                bool    `json:"enabled"`
	TemperatureSetpoint    float64 `json:"temperature_setpoint"`
	TemperatureSetpointMin float64 `json:"temperature_setpoint_min"`
	TemperatureSetpointMax float64 `json:"temperature_setpoint_max"`
	Mode                   string  `json:"mode"`
	FanSpeed               string  `json:"fan_speed"`
	AmbientTemperature     float64 `json:"ambient_temperature"`
	FaultCode              int     `json:"fault_code"`
	DeviceId               string  `json:"device_id"`
}

// Command payload format: {"value": ...}
type valueReq[T any] struct {
	Value *T `json:"value"`
}

func (c *Controller) onMessage(_ mqtt.Client, msg mqtt.Message) {
	// topic format: <base>/set/<field>
	t := msg.Topic()
	c.log.Debug("mqtt message received", "topic", t, "payload_len", len(msg.Payload()))
	// Request: <base>/get/<what>
	if what, ok := strings.CutPrefix(t, c.cfg.BaseTopic+"/get/"); ok {
		switch what {
		case "snapshot":
			c.publishSnapshot()
		}
		return
	}

	// Command: <base>/set/<field>
	if field, ok := strings.CutPrefix(t, c.cfg.BaseTopic+"/set/"); ok {
		payload := msg.Payload()

		// Dispatch by field
		switch field {
		case "enabled":
			v, err := decodeValueStrict[bool](payload)
			if err != nil {
				c.log.Warn("mqtt decode failed", "field", field, "err", err)
				return
			}
			c.svc.SetEnabled(v)

		case "temperature_setpoint":
			v, err := decodeValueStrict[float64](payload)
			if err != nil {
				c.log.Warn("mqtt decode failed", "field", field, "err", err)
				return
			}
			if err := c.svc.SetSetpoint(v); err != nil {
				c.log.Warn("mqtt set failed", "field", field, "err", err)
			}

		case "temperature_setpoint_min":
			v, err := decodeValueStrict[float64](payload)
			if err != nil {
				c.log.Warn("mqtt decode failed", "field", field, "err", err)
				return
			}
			cur := c.svc.Get()
			if err := c.svc.SetMinMax(v, cur.TemperatureSetpointMax); err != nil {
				c.log.Warn("mqtt set failed", "field", field, "err", err)
			}

		case "temperature_setpoint_max":
			v, err := decodeValueStrict[float64](payload)
			if err != nil {
				c.log.Warn("mqtt decode failed", "field", field, "err", err)
				return
			}
			cur := c.svc.Get()
			if err := c.svc.SetMinMax(cur.TemperatureSetpointMin, v); err != nil {
				c.log.Warn("mqtt set failed", "field", field, "err", err)
			}

		case "mode":
			s, err := decodeValueStrict[string](payload)
			if err != nil {
				c.log.Warn("mqtt decode failed", "field", field, "err", err)
				return
			}
			m, err := thermostat.ParseMode(s)
			if err != nil {
				c.log.Warn("mqtt parse failed", "field", field, "value", s, "err", err)
				return
			}
			if err := c.svc.SetMode(m); err != nil {
				c.log.Warn("mqtt set failed", "field", field, "err", err)
			}

		case "fan_speed":
			s, err := decodeValueStrict[string](payload)
			if err != nil {
				c.log.Warn("mqtt decode failed", "field", field, "err", err)
				return
			}
			f, err := thermostat.ParseFanSpeed(s)
			if err != nil {
				c.log.Warn("mqtt parse failed", "field", field, "value", s, "err", err)
				return
			}
			if err := c.svc.SetFanSpeed(f); err != nil {
				c.log.Warn("mqtt set failed", "field", field, "err", err)
			}

		case "fault_code":
			v, err := decodeValueStrict[int](payload)
			if err != nil {
				c.log.Warn("mqtt decode failed", "field", field, "err", err)
				return
			}
			c.svc.SetFaultCode(v)
		}
		c.publishSnapshot()
	}
}

func (c *Controller) topic(suffix string) string {
	return strings.TrimRight(c.cfg.BaseTopic, "/") + "/" + suffix
}

func decodeValueStrict[T any](b []byte) (T, error) {
	var zero T
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var req valueReq[T]
	if err := dec.Decode(&req); err != nil {
		return zero, err
	}
	if req.Value == nil {
		return zero, errors.New("missing field 'value'")
	}
	return *req.Value, nil
}
