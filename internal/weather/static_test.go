package weather

import (
	"context"
	"testing"
)

func TestStaticReturnsConfiguredTemperature(t *testing.T) {
	s := NewStatic(13.5)

	got, err := s.OutdoorTemperature(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 13.5 {
		t.Fatalf("got %v, want 13.5", got)
	}
}

func TestStaticIsStableAcrossCalls(t *testing.T) {
	s := NewStatic(-4)

	for i := 0; i < 3; i++ {
		got, err := s.OutdoorTemperature(context.Background())
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if got != -4 {
			t.Fatalf("call %d: got %v, want -4", i, got)
		}
	}
}
