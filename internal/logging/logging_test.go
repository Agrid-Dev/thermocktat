package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewRespectsLevelAndFormat(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		cfg           Config
		wantJSON      bool
		debugShouldBe string // "present" or "filtered"
	}{
		{"defaults", Config{}, false, "filtered"},
		{"debug text", Config{Level: "debug", Format: "text"}, false, "present"},
		{"info json", Config{Level: "info", Format: "json"}, true, "filtered"},
		{"debug json", Config{Level: "DEBUG", Format: "JSON"}, true, "present"},
		{"garbage falls back to info text", Config{Level: "yelling", Format: "yaml"}, false, "filtered"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			log := newWithWriter(tc.cfg, &buf)
			log.Debug("debug-line", "k", "v")
			log.Info("info-line", "k", "v")

			out := buf.String()
			if !strings.Contains(out, "info-line") {
				t.Fatalf("info log should always appear, got: %q", out)
			}
			switch tc.debugShouldBe {
			case "present":
				if !strings.Contains(out, "debug-line") {
					t.Fatalf("debug log should be present at this level, got: %q", out)
				}
			case "filtered":
				if strings.Contains(out, "debug-line") {
					t.Fatalf("debug log should be filtered, got: %q", out)
				}
			}
			isJSON := strings.Contains(out, `"msg":"info-line"`)
			if tc.wantJSON && !isJSON {
				t.Fatalf("expected JSON output, got: %q", out)
			}
			if !tc.wantJSON && isJSON {
				t.Fatalf("expected text output, got: %q", out)
			}
		})
	}
}
