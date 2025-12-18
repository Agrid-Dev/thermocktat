package mqttctrl

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Agrid-Dev/thermocktat/internal/testutil"
	"github.com/Agrid-Dev/thermocktat/internal/thermostat"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type fakeMessage struct {
	topic   string
	payload []byte
}

func (m fakeMessage) Duplicate() bool   { return false }
func (m fakeMessage) Qos() byte         { return 0 }
func (m fakeMessage) Retained() bool    { return false }
func (m fakeMessage) Topic() string     { return m.topic }
func (m fakeMessage) MessageID() uint16 { return 0 }
func (m fakeMessage) Payload() []byte   { return m.payload }
func (m fakeMessage) Ack()              {}

type fakeToken struct {
	err  error
	done chan struct{}
}

func (t fakeToken) Done() <-chan struct{} {
	if t.done == nil {
		t.done = make(chan struct{})
		close(t.done)
	}
	return t.done
}

func (t fakeToken) Wait() bool                       { return true }
func (t fakeToken) WaitTimeout(_ time.Duration) bool { return true }
func (t fakeToken) Error() error                     { return t.err }

type publishCall struct {
	topic   string
	qos     byte
	retain  bool
	payload []byte
}

type fakeClient struct {
	publishes []publishCall
}

func (c *fakeClient) IsConnected() bool      { return true }
func (c *fakeClient) IsConnectionOpen() bool { return true }
func (c *fakeClient) Connect() mqtt.Token    { return fakeToken{} }
func (c *fakeClient) Disconnect(_ uint)      {}
func (c *fakeClient) Publish(topic string, qos byte, retained bool, payload interface{}) mqtt.Token {
	var b []byte
	switch v := payload.(type) {
	case []byte:
		b = append([]byte(nil), v...)
	case string:
		b = []byte(v)
	default:
		// shouldn't happen in our controller, but keep it safe
		tmp, _ := json.Marshal(v)
		b = tmp
	}
	c.publishes = append(c.publishes, publishCall{
		topic: topic, qos: qos, retain: retained, payload: b,
	})
	return fakeToken{}
}
func (c *fakeClient) Subscribe(_ string, _ byte, _ mqtt.MessageHandler) mqtt.Token {
	return fakeToken{}
}
func (c *fakeClient) SubscribeMultiple(_ map[string]byte, _ mqtt.MessageHandler) mqtt.Token {
	return fakeToken{}
}
func (c *fakeClient) Unsubscribe(_ ...string) mqtt.Token       { return fakeToken{} }
func (c *fakeClient) AddRoute(_ string, _ mqtt.MessageHandler) {}
func (c *fakeClient) OptionsReader() mqtt.ClientOptionsReader  { return mqtt.ClientOptionsReader{} }

// ---- tests ----
func newDefaultSvc() *testutil.FakeThermostatService {
	return testutil.NewFakeThermostatService()
}

func TestNewDefaults(t *testing.T) {
	svc := newDefaultSvc()
	c, err := New(svc, Config{DeviceID: "room101"})
	if err != nil {
		t.Fatal(err)
	}

	if c.cfg.BrokerURL != "tcp://localhost:1883" {
		t.Fatalf("expected default BrokerURL, got %q", c.cfg.BrokerURL)
	}
	if c.cfg.BaseTopic != "thermocktat/room101" {
		t.Fatalf("expected default BaseTopic, got %q", c.cfg.BaseTopic)
	}
	if c.cfg.ClientID != "thermocktat-room101" {
		t.Fatalf("expected default ClientID, got %q", c.cfg.ClientID)
	}
	if c.cfg.PublishInterval != 1*time.Second {
		t.Fatalf("expected default PublishInterval, got %v", c.cfg.PublishInterval)
	}
}

func TestNewValidation(t *testing.T) {
	svc := newDefaultSvc()

	if _, err := New(svc, Config{}); err == nil {
		t.Fatal("expected error when DeviceID missing")
	}

	if _, err := New(svc, Config{DeviceID: "x", QoS: 2}); err == nil {
		t.Fatal("expected error when QoS > 1")
	}
}

func TestTopicJoin(t *testing.T) {
	svc := newDefaultSvc()
	c, err := New(svc, Config{DeviceID: "room101", BaseTopic: "thermocktat/room101/"})
	if err != nil {
		t.Fatal(err)
	}
	if got := c.topic("snapshot"); got != "thermocktat/room101/snapshot" {
		t.Fatalf("expected topic without double slashes, got %q", got)
	}
}

func TestDecodeValueStrict(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v, err := decodeValueStrict[float64]([]byte(`{"value": 12.5}`))
		if err != nil {
			t.Fatal(err)
		}
		if v != 12.5 {
			t.Fatalf("expected 12.5, got %v", v)
		}
	})

	t.Run("missing value", func(t *testing.T) {
		_, err := decodeValueStrict[bool]([]byte(`{}`))
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("unknown field rejected", func(t *testing.T) {
		_, err := decodeValueStrict[string]([]byte(`{"value":"heat","extra":1}`))
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := decodeValueStrict[string]([]byte(`{"value":`))
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestOnMessage_IgnoresWrongPrefix(t *testing.T) {
	svc := newDefaultSvc()
	c, err := New(svc, Config{DeviceID: "room101"})
	if err != nil {
		t.Fatal(err)
	}

	c.onMessage(nil, fakeMessage{
		topic:   "otherprefix/set/enabled",
		payload: []byte(`{"value":true}`),
	})

	if svc.SetEnabledCalled {
		t.Fatal("expected SetEnabled not called")
	}
}

func TestOnMessage_Enabled(t *testing.T) {
	svc := newDefaultSvc()
	c, _ := New(svc, Config{DeviceID: "room101"})
	fc := &fakeClient{}
	c.client = fc
	c.onMessage(nil, fakeMessage{
		topic:   "thermocktat/room101/set/enabled",
		payload: []byte(`{"value":false}`),
	})

	if !svc.SetEnabledCalled || svc.SetEnabledArg != false {
		t.Fatalf("expected SetEnabled(false), got called=%v arg=%v", svc.SetEnabledCalled, svc.SetEnabledArg)
	}
}

func TestOnMessage_Setpoint(t *testing.T) {
	svc := newDefaultSvc()
	c, _ := New(svc, Config{DeviceID: "room101"})
	fc := &fakeClient{}
	c.client = fc

	c.onMessage(nil, fakeMessage{
		topic:   "thermocktat/room101/set/temperature_setpoint",
		payload: []byte(`{"value":23.5}`),
	})

	if !svc.SetSetpointCalled || svc.SetSetpointArg != 23.5 {
		t.Fatalf("expected SetSetpoint(23.5), got called=%v arg=%v", svc.SetSetpointCalled, svc.SetSetpointArg)
	}
}

func TestOnMessage_MinMax(t *testing.T) {
	svc := newDefaultSvc()
	svc.S.TemperatureSetpointMin = 10
	svc.S.TemperatureSetpointMax = 30

	c, _ := New(svc, Config{DeviceID: "room101"})
	fc := &fakeClient{}
	c.client = fc

	c.onMessage(nil, fakeMessage{
		topic:   "thermocktat/room101/set/temperature_setpoint_min",
		payload: []byte(`{"value":12}`),
	})

	if !svc.SetMinMaxCalled || svc.SetMinMaxMin != 12 || svc.SetMinMaxMax != 30 {
		t.Fatalf("expected SetMinMax(12,30), got called=%v min=%v max=%v",
			svc.SetMinMaxCalled, svc.SetMinMaxMin, svc.SetMinMaxMax)
	}
}

func TestOnMessage_MaxMin(t *testing.T) {
	svc := newDefaultSvc()
	svc.S.TemperatureSetpointMin = 10
	svc.S.TemperatureSetpointMax = 30

	c, _ := New(svc, Config{DeviceID: "room101"})
	fc := &fakeClient{}
	c.client = fc
	c.onMessage(nil, fakeMessage{
		topic:   "thermocktat/room101/set/temperature_setpoint_max",
		payload: []byte(`{"value":28}`),
	})

	if !svc.SetMinMaxCalled || svc.SetMinMaxMin != 10 || svc.SetMinMaxMax != 28 {
		t.Fatalf("expected SetMinMax(10,28), got called=%v min=%v max=%v",
			svc.SetMinMaxCalled, svc.SetMinMaxMin, svc.SetMinMaxMax)
	}
}

func TestOnMessage_Mode(t *testing.T) {
	svc := newDefaultSvc()
	c, _ := New(svc, Config{DeviceID: "room101"})
	fc := &fakeClient{}
	c.client = fc
	c.onMessage(nil, fakeMessage{
		topic:   "thermocktat/room101/set/mode",
		payload: []byte(`{"value":"heat"}`),
	})

	if !svc.SetModeCalled || svc.SetModeArg != thermostat.ModeHeat {
		t.Fatalf("expected SetMode(Heat), got called=%v arg=%v", svc.SetModeCalled, svc.SetModeArg)
	}
}

func TestOnMessage_ModeInvalid_DoesNotCallService(t *testing.T) {
	svc := newDefaultSvc()
	c, _ := New(svc, Config{DeviceID: "room101"})
	fc := &fakeClient{}
	c.client = fc

	c.onMessage(nil, fakeMessage{
		topic:   "thermocktat/room101/set/mode",
		payload: []byte(`{"value":"weird"}`),
	})

	if svc.SetModeCalled {
		t.Fatal("expected SetMode not called")
	}
}

func TestOnMessage_FanSpeed(t *testing.T) {
	svc := newDefaultSvc()
	c, _ := New(svc, Config{DeviceID: "room101"})
	fc := &fakeClient{}
	c.client = fc
	c.onMessage(nil, fakeMessage{
		topic:   "thermocktat/room101/set/fan_speed",
		payload: []byte(`{"value":"high"}`),
	})

	if !svc.SetFanSpeedCalled || svc.SetFanSpeedArg != thermostat.FanHigh {
		t.Fatalf("expected SetFanSpeed(High), got called=%v arg=%v", svc.SetFanSpeedCalled, svc.SetFanSpeedArg)
	}
}

func TestOnMessage_FanSpeedInvalid_DoesNotCallService(t *testing.T) {
	svc := newDefaultSvc()
	c, _ := New(svc, Config{DeviceID: "room101"})
	fc := &fakeClient{}
	c.client = fc

	c.onMessage(nil, fakeMessage{
		topic:   "thermocktat/room101/set/fan_speed",
		payload: []byte(`{"value":"turbo"}`),
	})

	if svc.SetFanSpeedCalled {
		t.Fatal("expected SetFanSpeed not called")
	}
}

func TestPublishSnapshot_PublishesJSON(t *testing.T) {
	svc := newDefaultSvc()
	c, _ := New(svc, Config{DeviceID: "room101", QoS: 1, RetainSnapshot: true})

	fc := &fakeClient{}
	c.client = fc

	c.publishSnapshot()

	if len(fc.publishes) != 1 {
		t.Fatalf("expected 1 publish, got %d", len(fc.publishes))
	}

	p := fc.publishes[0]
	if p.topic != "thermocktat/room101/snapshot" {
		t.Fatalf("expected snapshot topic, got %q", p.topic)
	}
	if p.qos != 1 || p.retain != true {
		t.Fatalf("expected qos=1 retain=true, got qos=%d retain=%v", p.qos, p.retain)
	}

	var got map[string]any
	if err := json.Unmarshal(p.payload, &got); err != nil {
		t.Fatalf("invalid published json: %v payload=%s", err, string(p.payload))
	}
	if got["mode"] != "auto" {
		t.Fatalf("expected mode=auto, got %v", got["mode"])
	}
	if got["fan_speed"] != "auto" {
		t.Fatalf("expected fan_speed=auto, got %v", got["fan_speed"])
	}
}

// Optional: shows we ignore service errors (controller swallows them).
func TestOnMessage_ServiceError_IsIgnored(t *testing.T) {
	svc := newDefaultSvc()
	svc.SetSetpointErr = errors.New("boom")
	c, _ := New(svc, Config{DeviceID: "room101"})
	fc := &fakeClient{}
	c.client = fc
	c.onMessage(nil, fakeMessage{
		topic:   "thermocktat/room101/set/temperature_setpoint",
		payload: []byte(`{"value":25}`),
	})

	if !svc.SetSetpointCalled {
		t.Fatal("expected SetSetpoint called")
	}
}
