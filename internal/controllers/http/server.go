package httpctrl

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Agrid-Dev/thermocktat/internal/ports"
	"github.com/Agrid-Dev/thermocktat/internal/thermostat"
)

type Server struct {
	svc      ports.ThermostatService
	srv      *http.Server
	deviceID string
}

// New returns a runnable server.
func New(svc ports.ThermostatService, addr string, deviceID string) *Server {
	mux := http.NewServeMux()
	s := &Server{svc: svc, deviceID: deviceID}

	// Read
	mux.HandleFunc("GET /v1", s.handleGet)

	// Write: one endpoint per variable
	mux.HandleFunc("POST /v1/enabled", s.handlePostEnabled)
	mux.HandleFunc("POST /v1/temperature_setpoint", s.handlePostSetpoint)
	mux.HandleFunc("POST /v1/temperature_setpoint_min", s.handlePostMinSetpoint)
	mux.HandleFunc("POST /v1/temperature_setpoint_max", s.handlePostMaxSetpoint)
	mux.HandleFunc("POST /v1/mode", s.handlePostMode)
	mux.HandleFunc("POST /v1/fan_speed", s.handlePostFanSpeed)

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	s.srv = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(shutdownCtx)
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

// ---- DTOs ----

type snapshotDTO struct {
	DeviceID               string  `json:"device_id"`
	Enabled                bool    `json:"enabled"`
	TemperatureSetpoint    float64 `json:"temperature_setpoint"`
	TemperatureSetpointMin float64 `json:"temperature_setpoint_min"`
	TemperatureSetpointMax float64 `json:"temperature_setpoint_max"`
	Mode                   string  `json:"mode"`
	FanSpeed               string  `json:"fan_speed"`
	AmbientTemperature     float64 `json:"ambient_temperature"`
}

func toDTO(s thermostat.Snapshot) snapshotDTO {
	return snapshotDTO{
		Enabled:                s.Enabled,
		TemperatureSetpoint:    s.TemperatureSetpoint,
		TemperatureSetpointMin: s.TemperatureSetpointMin,
		TemperatureSetpointMax: s.TemperatureSetpointMax,
		Mode:                   s.Mode.String(),
		FanSpeed:               s.FanSpeed.String(),
		AmbientTemperature:     s.AmbientTemperature,
	}
}

// ---- Handlers ----

func (s *Server) handleGet(w http.ResponseWriter, _ *http.Request) {
	s.respondSnapshot(w)
}

func (s *Server) handlePostEnabled(w http.ResponseWriter, r *http.Request) {
	postValue(s, w, r, func(v bool) error {
		s.svc.SetEnabled(v)
		return nil
	})
}

func (s *Server) handlePostSetpoint(w http.ResponseWriter, r *http.Request) {
	postValue(s, w, r, func(v float64) error {
		return s.svc.SetSetpoint(v)
	})
}

func (s *Server) handlePostMinSetpoint(w http.ResponseWriter, r *http.Request) {
	postValue(s, w, r, func(v float64) error {
		cur := s.svc.Get()
		return s.svc.SetMinMax(v, cur.TemperatureSetpointMax)
	})
}

func (s *Server) handlePostMaxSetpoint(w http.ResponseWriter, r *http.Request) {
	postValue(s, w, r, func(v float64) error {
		cur := s.svc.Get()
		return s.svc.SetMinMax(cur.TemperatureSetpointMin, v)
	})
}

func (s *Server) handlePostMode(w http.ResponseWriter, r *http.Request) {
	// body: {"value": "heat"}
	postValue(s, w, r, func(v string) error {
		m, err := thermostat.ParseMode(v)
		if err != nil {
			return err
		}
		return s.svc.SetMode(m)
	})
}

func (s *Server) handlePostFanSpeed(w http.ResponseWriter, r *http.Request) {
	// body: {"value": "high"}
	postValue(s, w, r, func(v string) error {
		f, err := thermostat.ParseFanSpeed(v)
		if err != nil {
			return err
		}
		return s.svc.SetFanSpeed(f)
	})
}

// ---- generic helpers ----
func (s *Server) respondSnapshot(w http.ResponseWriter) {
	dto := toDTO(s.svc.Get())
	dto.DeviceID = s.deviceID
	writeJSON(w, http.StatusOK, dto)
}

func postValue[T any](s *Server, w http.ResponseWriter, r *http.Request, apply func(T) error) {
	dec := json.NewDecoder(r.Body)
	var req struct {
		Value *T `json:"value"`
	}
	if err := dec.Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Value == nil {
		writeErr(w, http.StatusBadRequest, "missing field 'value'")
		return
	}

	if err := apply(*req.Value); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	s.respondSnapshot(w)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
