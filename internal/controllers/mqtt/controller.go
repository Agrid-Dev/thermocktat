package mqttctrl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/Agrid-Dev/thermocktat/internal/ports"
	"github.com/Agrid-Dev/thermocktat/internal/thermostat"
	mqtt "github.com/eclipse/paho.mqtt.golang"
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

	Username string
	Password string
}

type Controller struct {
	svc ports.ThermostatService
	cfg Config

	client mqtt.Client
}

func New(svc ports.ThermostatService, cfg Config) (*Controller, error) {
	// ---- defaults ----

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
	if cfg.QoS > 1 {
		return nil, errors.New("mqtt: QoS must be 0 or 1")
	}
	return &Controller{
		svc: svc,
		cfg: cfg,
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
		// Subscribe to all set commands under BaseTopic.
		topic := c.topic("set/+")
		token := cl.Subscribe(topic, c.cfg.QoS, c.onMessage)
		token.Wait()
		// If subscribe fails, paho exposes token.Error().
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

	var last thermostat.Snapshot
	first := true

	// publish immediately once
	c.publishSnapshot()

	for {
		select {
		case <-ctx.Done():
			c.client.Disconnect(250)
			return ctx.Err()

		case <-ticker.C:
			cur := c.svc.Get()
			if first || !reflect.DeepEqual(cur, last) {
				c.publishSnapshot()
				last = cur
				first = false
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
}

// Command payload format: {"value": ...}
type valueReq[T any] struct {
	Value *T `json:"value"`
}

func (c *Controller) onMessage(_ mqtt.Client, msg mqtt.Message) {
	// topic format: <base>/set/<field>
	t := msg.Topic()
	prefix := c.cfg.BaseTopic + "/set/"
	if !strings.HasPrefix(t, prefix) {
		return
	}
	field := strings.TrimPrefix(t, prefix)

	payload := msg.Payload()

	// Dispatch by field
	switch field {
	case "enabled":
		v, err := decodeValueStrict[bool](payload)
		if err != nil {
			return
		}
		c.svc.SetEnabled(v)

	case "setpoint":
		v, err := decodeValueStrict[float64](payload)
		if err != nil {
			return
		}
		_ = c.svc.SetSetpoint(v)

	case "min_setpoint":
		v, err := decodeValueStrict[float64](payload)
		if err != nil {
			return
		}
		cur := c.svc.Get()
		_ = c.svc.SetMinMax(v, cur.TemperatureSetpointMax)

	case "max_setpoint":
		v, err := decodeValueStrict[float64](payload)
		if err != nil {
			return
		}
		cur := c.svc.Get()
		_ = c.svc.SetMinMax(cur.TemperatureSetpointMin, v)

	case "mode":
		s, err := decodeValueStrict[string](payload)
		if err != nil {
			return
		}
		m, err := thermostat.ParseMode(s)
		if err != nil {
			return
		}
		_ = c.svc.SetMode(m)

	case "fan_speed":
		s, err := decodeValueStrict[string](payload)
		if err != nil {
			return
		}
		f, err := thermostat.ParseFanSpeed(s)
		if err != nil {
			return
		}
		_ = c.svc.SetFanSpeed(f)
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
