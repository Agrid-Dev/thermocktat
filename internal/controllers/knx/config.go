package knxctrl

import "time"

// Config holds the KNX controller configuration.
type Config struct {
	DeviceID        string
	Addr            string
	PublishInterval time.Duration // how often to check for state changes and push updates
	GAMain          int           // group address main group (0–31)
	GAMiddle        int           // group address middle group (0–7)
}
