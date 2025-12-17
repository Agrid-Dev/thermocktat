package device

import (
	"testing"

	"github.com/Agrid-Dev/thermocktat/internal/thermostat"
)

func TestNewDevice(t *testing.T) {
	id := "test-id"
	thermostatInstance := &thermostat.Thermostat{}
	device := New(id, thermostatInstance)

	if device.ID != id {
		t.Errorf("Expected device ID to be %s, got %s", id, device.ID)
	}
}
