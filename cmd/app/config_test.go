package app

import "testing"

func TestEnvKeyTransform_TopLevel(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"DEVICE_ID", "device_id"},
		{"CONTROLLER", "controller"},
		{"ADDR", "addr"},
		{"", ""},
		{"   ", ""},
	}

	for _, tt := range tests {
		got := envKeyTransform(tt.in)
		if got != tt.want {
			t.Fatalf("envKeyTransform(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestEnvKeyTransform_Controllers(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"CONTROLLERS_HTTP_ADDR", "controllers.http.addr"},
		{"CONTROLLERS_MQTT_PUBLISH_INTERVAL", "controllers.mqtt.publish_interval"},
		{"CONTROLLERS_MODBUS_UNIT_ID", "controllers.modbus.unit_id"},
		{"CONTROLLERS_HTTP", "controllers_http"},   // not enough parts -> fallback
		{"CONTROLLERS__ADDR", "controllers..addr"}, // edge case
		{"controllers_HTTP_addr", "controllers.http.addr"},
		{"CONTROLLERS_MQTT_PUBLISH_MODE", "controllers.mqtt.publish_mode"},
	}

	for _, tt := range tests {
		got := envKeyTransform(tt.in)
		if got != tt.want {
			t.Fatalf("envKeyTransform(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestEnvKeyTransform_ThermostatAndRegulator(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"THERMOSTAT_TEMPERATURE_SETPOINT", "thermostat.temperature_setpoint"},
		{"THERMOSTAT_AMBIENT_TEMPERATURE", "thermostat.ambient_temperature"},
		{"REGULATOR_INTERVAL", "regulator.interval"},
		{"REGULATOR_TRIGGER_HYSTERESIS", "regulator.trigger_hysteresis"},
		{"THERMOSTAT", "thermostat"}, // not enough parts -> passthrough
		{"REGULATOR", "regulator"},   // not enough parts -> passthrough
		{"HEAT_LOSS_COEFFICIENT", "heat_loss.coefficient"},
		{"HEAT_LOSS_OUTDOOR_TEMPERATURE", "heat_loss.outdoor_temperature"},
	}

	for _, tt := range tests {
		got := envKeyTransform(tt.in)
		if got != tt.want {
			t.Fatalf("envKeyTransform(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
