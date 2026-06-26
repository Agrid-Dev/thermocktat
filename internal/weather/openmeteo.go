package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const (
	defaultOpenMeteoBaseURL = "https://api.open-meteo.com/v1/forecast"
	defaultRequestTimeout   = 5 * time.Second
)

type OpenMeteoConfig struct {
	Latitude  float64
	Longitude float64

	// RefreshInterval is the cache TTL: a fetched value is reused for this long
	// before the next network call. Zero fetches on every call.
	RefreshInterval time.Duration

	BaseURL    string // defaults to the public Open-Meteo endpoint
	HTTPClient *http.Client
	Logger     *slog.Logger
}

type OpenMeteo struct {
	latitude  float64
	longitude float64
	ttl       time.Duration
	baseURL   string
	client    *http.Client
	log       *slog.Logger

	mu        sync.Mutex
	last      float64
	hasLast   bool
	fetchedAt time.Time
	now       func() time.Time
}

// openMeteoResponse is the subset of the forecast payload we read.
type openMeteoResponse struct {
	Current struct {
		Time          string  `json:"time"`
		Temperature2m float64 `json:"temperature_2m"`
	} `json:"current"`
	CurrentUnits struct {
		Temperature2m string `json:"temperature_2m"`
	} `json:"current_units"`
}

func NewOpenMeteo(cfg OpenMeteoConfig) *OpenMeteo {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultOpenMeteoBaseURL
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultRequestTimeout}
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &OpenMeteo{
		latitude:  cfg.Latitude,
		longitude: cfg.Longitude,
		ttl:       cfg.RefreshInterval,
		baseURL:   baseURL,
		client:    client,
		log:       logger,
		now:       time.Now,
	}
}

// OutdoorTemperature serves the cached value while fresh; on a failed refresh
// it keeps serving the last known good value and only errors when there is none.
func (o *OpenMeteo) OutdoorTemperature(ctx context.Context) (float64, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.hasLast && o.now().Sub(o.fetchedAt) < o.ttl {
		return o.last, nil
	}

	temp, err := o.fetch(ctx)
	if err != nil {
		if o.hasLast {
			o.log.Warn("open-meteo refresh failed, serving last known value",
				"err", err,
				"temperature", o.last,
			)
			return o.last, nil
		}
		return 0, err
	}

	o.last = temp
	o.hasLast = true
	o.fetchedAt = o.now()
	return temp, nil
}

func (o *OpenMeteo) fetch(ctx context.Context) (float64, error) {
	endpoint, err := o.requestURL()
	if err != nil {
		return 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("build open-meteo request: %w", err)
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("open-meteo request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return 0, fmt.Errorf("open-meteo returned status %d: %s", resp.StatusCode, body)
	}

	var payload openMeteoResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, fmt.Errorf("decode open-meteo response: %w", err)
	}

	o.log.Info("open-meteo query result",
		"latitude", o.latitude,
		"longitude", o.longitude,
		"temperature", payload.Current.Temperature2m,
		"unit", payload.CurrentUnits.Temperature2m,
		"observed_at", payload.Current.Time,
	)

	return payload.Current.Temperature2m, nil
}

func (o *OpenMeteo) requestURL() (string, error) {
	u, err := url.Parse(o.baseURL)
	if err != nil {
		return "", fmt.Errorf("parse open-meteo base url: %w", err)
	}
	q := u.Query()
	q.Set("latitude", strconv.FormatFloat(o.latitude, 'f', -1, 64))
	q.Set("longitude", strconv.FormatFloat(o.longitude, 'f', -1, 64))
	q.Set("current", "temperature_2m")
	u.RawQuery = q.Encode()
	return u.String(), nil
}
