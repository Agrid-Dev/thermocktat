package knxctrl

import (
	"testing"
)

func TestGroupAddress(t *testing.T) {
	cases := []struct {
		main, middle, sub int
		want              uint16
	}{
		{0, 0, 0, 0x0000},
		{1, 0, 0, 0x0800},
		{1, 0, 1, 0x0801},
		{1, 0, 6, 0x0806},
		{31, 7, 255, 0xFFFF},
	}
	for _, tc := range cases {
		got := GroupAddress(tc.main, tc.middle, tc.sub)
		if got != tc.want {
			t.Fatalf("%d/%d/%d: got 0x%04X, want 0x%04X", tc.main, tc.middle, tc.sub, got, tc.want)
		}
	}
}

func TestBuildBindingMap(t *testing.T) {
	cfg := Config{GAMain: 1, GAMiddle: 0}
	m, err := BuildBindingMap(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m) != 7 {
		t.Fatalf("expected 7 bindings, got %d", len(m))
	}

	// Verify enabled is compact (DPTSize=0).
	b, ok := m[GroupAddress(1, 0, SubEnabled)]
	if !ok {
		t.Fatal("missing binding for enabled")
	}
	if b.DPTSize != 0 {
		t.Fatalf("enabled DPTSize: got %d, want 0", b.DPTSize)
	}

	// Verify ambient_temperature is read-only.
	b, ok = m[GroupAddress(1, 0, SubAmbientTemperature)]
	if !ok {
		t.Fatal("missing binding for ambient_temperature")
	}
	if b.Write != nil {
		t.Fatal("ambient_temperature should be read-only")
	}
}

func TestBuildBindingMap_InvalidMain(t *testing.T) {
	_, err := BuildBindingMap(Config{GAMain: 32, GAMiddle: 0})
	if err == nil {
		t.Fatal("expected error for main=32")
	}
}

func TestBuildBindingMap_InvalidMiddle(t *testing.T) {
	_, err := BuildBindingMap(Config{GAMain: 1, GAMiddle: 8})
	if err == nil {
		t.Fatal("expected error for middle=8")
	}
}
