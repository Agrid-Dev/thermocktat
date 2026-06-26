package weather

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const okBody = `{
  "current_units": {"temperature_2m": "°C"},
  "current": {"time": "2026-06-26T14:00", "temperature_2m": 28.3}
}`

func TestOpenMeteoHappyPath(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(okBody))
	}))
	defer srv.Close()

	p := NewOpenMeteo(OpenMeteoConfig{Latitude: 48.8566, Longitude: 2.3522, BaseURL: srv.URL})

	got, err := p.OutdoorTemperature(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 28.3 {
		t.Fatalf("got %v, want 28.3", got)
	}

	for _, want := range []string{"latitude=48.8566", "longitude=2.3522", "current=temperature_2m"} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("query %q missing %q", gotQuery, want)
		}
	}
}

func TestOpenMeteoCachesWithinTTL(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte(okBody))
	}))
	defer srv.Close()

	p := NewOpenMeteo(OpenMeteoConfig{BaseURL: srv.URL, RefreshInterval: time.Hour})

	for i := 0; i < 3; i++ {
		if _, err := p.OutdoorTemperature(context.Background()); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if hits != 1 {
		t.Fatalf("expected 1 upstream hit within TTL, got %d", hits)
	}
}

func TestOpenMeteoHTTPErrorWithoutCache(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := NewOpenMeteo(OpenMeteoConfig{BaseURL: srv.URL})

	if _, err := p.OutdoorTemperature(context.Background()); err == nil {
		t.Fatal("expected error when upstream fails and no cache exists")
	}
}

func TestOpenMeteoMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	p := NewOpenMeteo(OpenMeteoConfig{BaseURL: srv.URL})

	if _, err := p.OutdoorTemperature(context.Background()); err == nil {
		t.Fatal("expected error decoding malformed JSON")
	}
}

func TestOpenMeteoStaleFallbackOnError(t *testing.T) {
	var fail bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(okBody))
	}))
	defer srv.Close()

	// TTL 0 => every call attempts a fetch, so the second call really hits the
	// (now failing) server and must fall back to the cached value.
	p := NewOpenMeteo(OpenMeteoConfig{BaseURL: srv.URL, RefreshInterval: 0})

	first, err := p.OutdoorTemperature(context.Background())
	if err != nil || first != 28.3 {
		t.Fatalf("first call: got %v, err %v", first, err)
	}

	fail = true
	second, err := p.OutdoorTemperature(context.Background())
	if err != nil {
		t.Fatalf("second call should fall back, got err %v", err)
	}
	if second != 28.3 {
		t.Fatalf("expected stale value 28.3, got %v", second)
	}
}
