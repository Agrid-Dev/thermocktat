package testutil

import (
	"context"
	"sync"

	"github.com/Agrid-Dev/thermocktat/internal/thermostat"
)

var _ thermostat.WeatherProvider = (*FakeWeatherProvider)(nil)

// FakeWeatherProvider returns Temps in sequence, repeating the last one once
// exhausted, or Err when set. Safe for concurrent use.
type FakeWeatherProvider struct {
	mu    sync.Mutex
	Temps []float64
	Err   error

	Calls int
}

// NewFakeWeatherProvider returns a provider that always reports temp.
func NewFakeWeatherProvider(temp float64) *FakeWeatherProvider {
	return &FakeWeatherProvider{Temps: []float64{temp}}
}

// OutdoorTemperature returns the next temperature in the sequence, or Err if set.
func (f *FakeWeatherProvider) OutdoorTemperature(context.Context) (float64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	idx := f.Calls
	f.Calls++

	if f.Err != nil {
		return 0, f.Err
	}
	if len(f.Temps) == 0 {
		return 0, nil
	}
	if idx >= len(f.Temps) {
		idx = len(f.Temps) - 1
	}
	return f.Temps[idx], nil
}

// CallCount reads Calls under the lock, for use while the provider runs concurrently.
func (f *FakeWeatherProvider) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Calls
}
