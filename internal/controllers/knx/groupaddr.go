package knxctrl

import (
	"fmt"

	"github.com/Agrid-Dev/thermocktat/internal/ports"
	"github.com/Agrid-Dev/thermocktat/internal/thermostat"
)

// Hardcoded sub-addresses for each thermostat datapoint.
const (
	SubEnabled            = 0
	SubSetpoint           = 1
	SubSetpointMin        = 2
	SubSetpointMax        = 3
	SubAmbientTemperature = 4
	SubMode               = 5
	SubFanSpeed           = 6
)

// GroupAddress computes a 3-level group address from main/middle/sub.
// Layout: [MMMMMIII SSSSSSSS] where M=main(5 bits), I=middle(3 bits), S=sub(8 bits).
func GroupAddress(main, middle, sub int) uint16 {
	return uint16(main<<11 | middle<<8 | sub)
}

// Binding defines how a group address maps to the thermostat.
type Binding struct {
	// DPTSize is the data payload size in bytes.
	// 0 means compact encoding (DPT 1.001: value in APCI low bits).
	DPTSize int
	Read    func(thermostat.Snapshot) []byte
	Write   func(ports.ThermostatService, []byte) error // nil = read-only
}

// BuildBindingMap creates the GA→Binding map from the configured main/middle groups.
func BuildBindingMap(cfg Config) (map[uint16]Binding, error) {
	if cfg.GAMain < 0 || cfg.GAMain > 31 {
		return nil, fmt.Errorf("ga_main %d out of range 0–31", cfg.GAMain)
	}
	if cfg.GAMiddle < 0 || cfg.GAMiddle > 7 {
		return nil, fmt.Errorf("ga_middle %d out of range 0–7", cfg.GAMiddle)
	}

	ga := func(sub int) uint16 { return GroupAddress(cfg.GAMain, cfg.GAMiddle, sub) }

	return map[uint16]Binding{
		ga(SubEnabled): {
			DPTSize: 0, // compact
			Read: func(s thermostat.Snapshot) []byte {
				return []byte{EncodeDPT1(s.Enabled)}
			},
			Write: func(svc ports.ThermostatService, data []byte) error {
				if len(data) < 1 {
					return fmt.Errorf("DPT 1.001: missing data")
				}
				svc.SetEnabled(DecodeDPT1(data[0]))
				return nil
			},
		},
		ga(SubSetpoint): {
			DPTSize: 2,
			Read: func(s thermostat.Snapshot) []byte {
				b := EncodeDPT9(s.TemperatureSetpoint)
				return b[:]
			},
			Write: func(svc ports.ThermostatService, data []byte) error {
				if len(data) < 2 {
					return fmt.Errorf("DPT 9.001: need 2 bytes")
				}
				return svc.SetSetpoint(DecodeDPT9([2]byte{data[0], data[1]}))
			},
		},
		ga(SubSetpointMin): {
			DPTSize: 2,
			Read: func(s thermostat.Snapshot) []byte {
				b := EncodeDPT9(s.TemperatureSetpointMin)
				return b[:]
			},
			Write: func(svc ports.ThermostatService, data []byte) error {
				if len(data) < 2 {
					return fmt.Errorf("DPT 9.001: need 2 bytes")
				}
				cur := svc.Get()
				return svc.SetMinMax(DecodeDPT9([2]byte{data[0], data[1]}), cur.TemperatureSetpointMax)
			},
		},
		ga(SubSetpointMax): {
			DPTSize: 2,
			Read: func(s thermostat.Snapshot) []byte {
				b := EncodeDPT9(s.TemperatureSetpointMax)
				return b[:]
			},
			Write: func(svc ports.ThermostatService, data []byte) error {
				if len(data) < 2 {
					return fmt.Errorf("DPT 9.001: need 2 bytes")
				}
				cur := svc.Get()
				return svc.SetMinMax(cur.TemperatureSetpointMin, DecodeDPT9([2]byte{data[0], data[1]}))
			},
		},
		ga(SubAmbientTemperature): {
			DPTSize: 2,
			Read: func(s thermostat.Snapshot) []byte {
				b := EncodeDPT9(s.AmbientTemperature)
				return b[:]
			},
			Write: nil, // read-only
		},
		ga(SubMode): {
			DPTSize: 1,
			Read: func(s thermostat.Snapshot) []byte {
				return []byte{byte(s.Mode)}
			},
			Write: func(svc ports.ThermostatService, data []byte) error {
				if len(data) < 1 {
					return fmt.Errorf("DPT 20.102: missing data")
				}
				return svc.SetMode(thermostat.Mode(data[0]))
			},
		},
		ga(SubFanSpeed): {
			DPTSize: 1,
			Read: func(s thermostat.Snapshot) []byte {
				return []byte{byte(s.FanSpeed)}
			},
			Write: func(svc ports.ThermostatService, data []byte) error {
				if len(data) < 1 {
					return fmt.Errorf("DPT 5.010: missing data")
				}
				return svc.SetFanSpeed(thermostat.FanSpeed(data[0]))
			},
		},
	}, nil
}
