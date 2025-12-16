package thermostat

import "testing"

func TestModeValid(t *testing.T) {
	cases := []struct {
		m    Mode
		want bool
	}{
		{ModeUnknown, false},
		{ModeHeat, true},
		{ModeCool, true},
		{ModeFan, true},
		{ModeAuto, true},
		{Mode(999), false},
	}

	for _, tc := range cases {
		if got := tc.m.Valid(); got != tc.want {
			t.Fatalf("Mode(%d).Valid()=%v want %v", tc.m, got, tc.want)
		}
	}
}

func TestModeString_Table(t *testing.T) {
	cases := []struct {
		name string
		in   Mode
		want string
	}{
		{"unknown (zero)", ModeUnknown, "unknown"},
		{"heat", ModeHeat, "heat"},
		{"cool", ModeCool, "cool"},
		{"fan", ModeFan, "fan"},
		{"auto", ModeAuto, "auto"},
		{"unknown (out of range)", Mode(999), "unknown"},
		{"unknown (negative)", Mode(-1), "unknown"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.String(); got != tc.want {
				t.Fatalf("Mode(%d).String()=%q want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseMode_Table(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    Mode
		wantErr bool
	}{
		{"heat", "heat", ModeHeat, false},
		{"cool", "cool", ModeCool, false},
		{"fan", "fan", ModeFan, false},
		{"auto", "auto", ModeAuto, false},
		{"invalid", "nope", ModeUnknown, true},
		{"empty", "", ModeUnknown, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseMode(tc.in)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseMode(%q) expected error, got nil (mode=%v)", tc.in, got)
				}
				// for invalid inputs we also expect unknown
				if got != tc.want {
					t.Fatalf("ParseMode(%q)=%v want %v", tc.in, got, tc.want)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseMode(%q) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("ParseMode(%q)=%v want %v", tc.in, got, tc.want)
			}
		})
	}
}
func TestFanSpeedValid(t *testing.T) {
	cases := []struct {
		f    FanSpeed
		want bool
	}{
		{FanUnknown, false},
		{FanAuto, true},
		{FanLow, true},
		{FanMedium, true},
		{FanHigh, true},
		{FanSpeed(-1), false},
	}

	for _, tc := range cases {
		if got := tc.f.Valid(); got != tc.want {
			t.Fatalf("FanSpeed(%d).Valid()=%v want %v", tc.f, got, tc.want)
		}
	}
}

func TestFanSpeedString_Table(t *testing.T) {
	cases := []struct {
		name string
		in   FanSpeed
		want string
	}{
		{"unknown (zero)", FanUnknown, "unknown"},
		{"auto", FanAuto, "auto"},
		{"low", FanLow, "low"},
		{"medium", FanMedium, "medium"},
		{"high", FanHigh, "high"},
		{"unknown (out of range)", FanSpeed(999), "unknown"},
		{"unknown (negative)", FanSpeed(-1), "unknown"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.String(); got != tc.want {
				t.Fatalf("FanSpeed(%d).String()=%q want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseFanSpeed_Table(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    FanSpeed
		wantErr bool
	}{
		{"auto", "auto", FanAuto, false},
		{"low", "low", FanLow, false},
		{"medium", "medium", FanMedium, false},
		{"high", "high", FanHigh, false},
		{"invalid", "nope", FanUnknown, true},
		{"empty", "", FanUnknown, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseFanSpeed(tc.in)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseFanSpeed(%q) expected error, got nil (fan=%v)", tc.in, got)
				}
				if got != tc.want {
					t.Fatalf("ParseFanSpeed(%q)=%v want %v", tc.in, got, tc.want)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseFanSpeed(%q) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("ParseFanSpeed(%q)=%v want %v", tc.in, got, tc.want)
			}
		})
	}
}
