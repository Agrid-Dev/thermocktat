package httpctrl

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Agrid-Dev/thermocktat/internal/thermostat"
)

type fakeSvc struct {
	s thermostat.Snapshot

	setModeCalled bool
	setModeArg    thermostat.Mode
	setModeErr    error

	setFanCalled bool
	setFanArg    thermostat.FanSpeed
	setFanErr    error

	setSetpointCalled bool
	setSetpointArg    float64
	setSetpointErr    error
	setMinMaxErr      error
}

func (f *fakeSvc) Get() thermostat.Snapshot { return f.s }
func (f *fakeSvc) SetEnabled(b bool)        { f.s.Enabled = b }
func (f *fakeSvc) SetMinMax(min, max float64) error {
	f.s.TemperatureSetpointMin = min
	f.s.TemperatureSetpointMax = max
	if f.setMinMaxErr != nil {
		return f.setMinMaxErr
	}
	return nil
}
func (f *fakeSvc) SetMode(m thermostat.Mode) error {
	f.setModeCalled = true
	f.setModeArg = m
	if f.setModeErr != nil {
		return f.setModeErr
	}
	f.s.Mode = m
	return nil
}
func (f *fakeSvc) SetFanSpeed(fs thermostat.FanSpeed) error {
	f.setFanCalled = true
	f.setFanArg = fs
	if f.setFanErr != nil {
		return f.setFanErr
	}
	f.s.FanSpeed = fs
	return nil
}
func (f *fakeSvc) SetSetpoint(sp float64) error {
	f.setSetpointCalled = true
	f.setSetpointArg = sp
	if f.setSetpointErr != nil {
		return f.setSetpointErr
	}
	f.s.TemperatureSetpoint = sp
	return nil
}

func TestGET_v1_ReturnsStrings(t *testing.T) {
	srv, _ := newTestServer()

	rr := doJSONRequest(t, srv.srv.Handler, http.MethodGet, "/v1", nil)
	assertStatus(t, rr, http.StatusOK)

	got := decodeJSON[map[string]any](t, rr)
	if got["mode"] != "auto" {
		t.Fatalf("expected mode=auto, got %v", got["mode"])
	}
	if got["fan_speed"] != "auto" {
		t.Fatalf("expected fan_speed=auto, got %v", got["fan_speed"])
	}
}

func TestPOST_mode_Valid(t *testing.T) {
	srv, f := newTestServer()

	rr := doJSONRequest(t, srv.srv.Handler, http.MethodPost, "/v1/mode", map[string]any{
		"value": "heat",
	})
	assertStatus(t, rr, http.StatusOK)

	if !f.setModeCalled || f.setModeArg != thermostat.ModeHeat {
		t.Fatalf("expected SetMode(Heat) called, got called=%v arg=%v", f.setModeCalled, f.setModeArg)
	}
}

func TestPOST_mode_InvalidPayload(t *testing.T) {
	srv, _ := newTestServer()

	// Wrong key => Value missing (your current handler returns missing field 'value')
	rr := doJSONRequest(t, srv.srv.Handler, http.MethodPost, "/v1/mode", map[string]any{
		"mode": "weird",
	})
	assertStatus(t, rr, http.StatusBadRequest)
	_ = assertErrorResponse(t, rr)
}

func TestPOST_mode_InvalidString(t *testing.T) {
	srv, _ := newTestServer()

	rr := doJSONRequest(t, srv.srv.Handler, http.MethodPost, "/v1/mode", map[string]any{
		"value": "weird",
	})
	assertStatus(t, rr, http.StatusBadRequest)
	_ = assertErrorResponse(t, rr)
}

func TestPOST_setpoint_ErrorFromService(t *testing.T) {
	srv, f := newTestServer()
	f.setSetpointErr = thermostat.ErrSetpointOutOfRange

	rr := doJSONRequest(t, srv.srv.Handler, http.MethodPost, "/v1/setpoint", map[string]any{
		"value": 999,
	})
	assertStatus(t, rr, http.StatusBadRequest)
	_ = assertErrorResponse(t, rr)
}

func TestPOST_enabled(t *testing.T) {
	srv, f := newTestServer()

	rr := postValueEndpoint(t, srv, "/v1/enabled", false)
	assertStatus(t, rr, http.StatusOK)

	if f.s.Enabled != false {
		t.Fatalf("expected enabled=false, got %v", f.s.Enabled)
	}
}

func TestPOST_fan_speed(t *testing.T) {
	srv, f := newTestServer()

	rr := postValueEndpoint(t, srv, "/v1/fan_speed", "high")
	assertStatus(t, rr, http.StatusOK)

	if !f.setFanCalled || f.setFanArg != thermostat.FanHigh {
		t.Fatalf("expected SetFanSpeed(High), got called=%v arg=%v", f.setFanCalled, f.setFanArg)
	}
}

func TestPOST_min_setpoint(t *testing.T) {
	srv, f := newTestServer()

	// Test successful min setpoint update
	rr := postValueEndpoint(t, srv, "/v1/min_setpoint", 18.0)
	assertStatus(t, rr, http.StatusOK)

	if f.s.TemperatureSetpointMin != 18.0 {
		t.Fatalf("expected min setpoint=18.0, got %v", f.s.TemperatureSetpointMin)
	}

	// Test invalid min setpoint (greater than current max)
	f.setMinMaxErr = thermostat.ErrInvalidMinMax
	rr = postValueEndpoint(t, srv, "/v1/min_setpoint", 30.0)
	assertStatus(t, rr, http.StatusBadRequest)
	_ = assertErrorResponse(t, rr)
}

func TestPOST_max_setpoint(t *testing.T) {
	srv, f := newTestServer()

	// Test successful max setpoint update
	rr := postValueEndpoint(t, srv, "/v1/max_setpoint", 26.0)
	assertStatus(t, rr, http.StatusOK)

	if f.s.TemperatureSetpointMax != 26.0 {
		t.Fatalf("expected max setpoint=26.0, got %v", f.s.TemperatureSetpointMax)
	}

	// Test invalid max setpoint (less than current min)
	f.setMinMaxErr = thermostat.ErrInvalidMinMax
	rr = postValueEndpoint(t, srv, "/v1/max_setpoint", 15.0)
	assertStatus(t, rr, http.StatusBadRequest)
	_ = assertErrorResponse(t, rr)
}

func TestGET_healthz(t *testing.T) {
	srv, _ := newTestServer()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	srv.srv.Handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)
	if rr.Body.String() != "ok" {
		t.Fatalf("expected body 'ok', got %s", rr.Body.String())
	}
}

// ---- test helpers ----

func newFakeSvc() *fakeSvc {
	return &fakeSvc{
		s: thermostat.Snapshot{
			Enabled:                true,
			TemperatureSetpoint:    22,
			TemperatureSetpointMin: 16,
			TemperatureSetpointMax: 28,
			Mode:                   thermostat.ModeAuto,
			FanSpeed:               thermostat.FanAuto,
			AmbientTemperature:     21,
		},
	}
}

func newTestServer() (*Server, *fakeSvc) {
	f := newFakeSvc()
	DeviceId := "default"
	return New(f, ":0", DeviceId), f
}

func doJSONRequest(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var r *http.Request
	if body == nil {
		r = httptest.NewRequest(method, path, nil)
	} else {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		r = httptest.NewRequest(method, path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
	}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	return rr
}

func assertStatus(t *testing.T, rr *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rr.Code != want {
		t.Fatalf("expected %d, got %d body=%s", want, rr.Code, rr.Body.String())
	}
}

func decodeJSON[T any](t *testing.T, rr *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rr.Body.Bytes(), &v); err != nil {
		t.Fatalf("json.Unmarshal: %v body=%s", err, rr.Body.String())
	}
	return v
}

// Handy when you only care about error responses.
func assertErrorResponse(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var resp struct {
		Error string `json:"error"`
	}
	resp = decodeJSON[struct {
		Error string `json:"error"`
	}](t, rr)
	if resp.Error == "" {
		t.Fatalf("expected non-empty error field, got body=%s", rr.Body.String())
	}
	return resp.Error
}

func postValueEndpoint[T any](t *testing.T, srv *Server, path string, value T) *httptest.ResponseRecorder {
	t.Helper()
	return doJSONRequest(t, srv.srv.Handler, http.MethodPost, path, struct {
		Value T `json:"value"`
	}{Value: value})
}
