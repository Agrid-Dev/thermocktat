package knxctrl

import (
	"math"
	"testing"
)

func TestEncodeDPT9_RoundTrip(t *testing.T) {
	cases := []struct {
		name    string
		value   float64
		epsilon float64
	}{
		{"zero", 0.0, 0.01},
		{"positive", 22.0, 0.1},
		{"negative", -10.5, 0.1},
		{"large", 670760.0, 700.0}, // near max
		{"small positive", 0.01, 0.01},
		{"typical setpoint", 21.5, 0.1},
		{"typical ambient", 19.84, 0.1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded := EncodeDPT9(tc.value)
			decoded := DecodeDPT9(encoded)
			if math.Abs(decoded-tc.value) > tc.epsilon {
				t.Fatalf("round-trip failed: input=%f, got=%f (diff=%f, max=%f)",
					tc.value, decoded, math.Abs(decoded-tc.value), tc.epsilon)
			}
		})
	}
}

func TestDecodeDPT9_KnownValues(t *testing.T) {
	// Encode 22.0 and verify round-trip.
	encoded := EncodeDPT9(22.0)
	val := DecodeDPT9(encoded)
	if math.Abs(val-22.0) > 0.1 {
		t.Fatalf("expected ~22.0, got %f", val)
	}
}

func TestEncodeDPT1(t *testing.T) {
	if EncodeDPT1(true) != 1 {
		t.Fatal("true should encode to 1")
	}
	if EncodeDPT1(false) != 0 {
		t.Fatal("false should encode to 0")
	}
}

func TestDecodeDPT1(t *testing.T) {
	if !DecodeDPT1(1) {
		t.Fatal("1 should decode to true")
	}
	if DecodeDPT1(0) {
		t.Fatal("0 should decode to false")
	}
	// Bit masking: only low bit matters.
	if !DecodeDPT1(0x03) {
		t.Fatal("0x03 should decode to true (low bit set)")
	}
}
