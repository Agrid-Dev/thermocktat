package bacnetctrl

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/Agrid-Dev/thermocktat/internal/ports"
	"github.com/Agrid-Dev/thermocktat/internal/thermostat"
	"github.com/ulbios/bacnet"
	"github.com/ulbios/bacnet/objects"
	"github.com/ulbios/bacnet/plumbing"
	"github.com/ulbios/bacnet/services"
)

// Config for the BACnet controller.
type Config struct {
	// Identity
	DeviceID       string
	DeviceInstance int // BACnet device instance number (0..4194303)

	// Network
	Addr string // bind address, e.g. "0.0.0.0:47808" or "127.0.0.1:0" for ephemeral port in tests

	// Behavior
	SyncInterval time.Duration // retained for parity; unused by WhoIs handling
}

// BACnet object definitions for thermostat properties
const (
	// Object types (using numeric values since not all are defined in the library)
	objTypeAnalogInput     = 0  // Analog Input
	objTypeAnalogValue     = 2  // Analog Value
	objTypeBinaryInput     = 3  // Binary Input
	objTypeMultiStateValue = 19 // Multi-state Value

	// Property IDs
	propPresentValue = objects.PropertyIdPresentValue // 85

	// Object instance numbers (arbitrary but consistent)
	instAnalogInputAmbientTemp  = 0
	instAnalogValueSetpoint     = 1
	instAnalogValueSetpointMin  = 2
	instAnalogValueSetpointMax  = 3
	instBinaryInputEnabled      = 4
	instMultiStateValueMode     = 5
	instMultiStateValueFanSpeed = 6
)

type Controller struct {
	svc ports.ThermostatService
	cfg Config

	mu   sync.Mutex
	conn net.PacketConn
}

// New creates a BACnet controller. DeviceInstance must be in the BACnet range 0..4194303.
func New(svc ports.ThermostatService, cfg Config) (*Controller, error) {
	if cfg.DeviceID == "" {
		return nil, errors.New("bacnet: DeviceID is required")
	}
	if cfg.DeviceInstance < 0 || cfg.DeviceInstance > 4194303 {
		return nil, errors.New("bacnet: DeviceInstance must be in range 0..4194303")
	}
	if cfg.Addr == "" {
		cfg.Addr = "0.0.0.0:47808"
	}
	if cfg.SyncInterval <= 0 {
		cfg.SyncInterval = 1 * time.Second
	}
	return &Controller{svc: svc, cfg: cfg}, nil
}

// Run starts a BACnet/IP listener and responds to Who-Is messages with I-Am.
// It blocks until ctx is canceled.
func (c *Controller) Run(ctx context.Context) error {
	ln, err := net.ListenPacket("udp4", c.cfg.Addr)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.conn = ln
	c.mu.Unlock()

	// ensure connection is closed when function returns
	defer func() {
		c.mu.Lock()
		if c.conn != nil {
			_ = c.conn.Close()
			c.conn = nil
		}
		c.mu.Unlock()
	}()

	// Read loop goroutine
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 2048)
		for {
			n, addr, err := ln.ReadFrom(buf)
			if err != nil {
				// If the listener closed due to context cancel, exit loop.
				select {
				case <-ctx.Done():
					return
				default:
					// transient read error: log and continue.
					log.Printf("bacnet read error: %v", err)
					continue
				}
			}

			data := make([]byte, n)
			copy(data, buf[:n])

			peek := data
			if len(peek) > 32 {
				peek = peek[:32]
			}
			log.Printf("bacnet: received %d bytes from %s: % x", n, addr.String(), peek)
			// Parse the incoming BACnet message. Ignore parse errors (malformed packets).
			pkt, err := bacnet.Parse(data)
			if err != nil {
				// ignore non-BACnet or malformed packets
				log.Printf("bacnet: parse error from %s: %v (raw: % x)", addr.String(), err, peek)
				continue
			}

			// respond to WhoIs, ReadProperty, WriteProperty
			switch pkt := pkt.(type) {
			case *services.UnconfirmedWhoIs:
				log.Printf("bacnet: Who-Is received from %s — replying I-Am (device instance %d)", addr.String(), c.cfg.DeviceInstance)
				if err := c.respondIAm(addr); err != nil {
					log.Printf("bacnet: failed to send I-Am to %s: %v", addr.String(), err)
				}
			case *services.ConfirmedReadProperty:
				log.Printf("bacnet: ReadProperty request received from %s", addr.String())
				if err := c.respondReadProperty(addr, pkt); err != nil {
					log.Printf("bacnet: failed to respond to ReadProperty from %s: %v", addr.String(), err)
				}
			case *services.ConfirmedWriteProperty:
				log.Printf("bacnet: WriteProperty request received from %s", addr.String())
				if err := c.respondWriteProperty(addr, pkt); err != nil {
					log.Printf("bacnet: failed to respond to WriteProperty from %s: %v", addr.String(), err)
				}
			default:
				// other services not handled here
				log.Printf("bacnet: received unsupported service type from %s: %T", addr.String(), pkt)

			}
		}
	}()

	// Wait until context done, then close listener and wait for read loop to finish.
	<-ctx.Done()
	_ = ln.Close()
	<-readDone
	return ctx.Err()
}

// respondIAm builds an Unconfirmed I-Am and sends it to the given addr.
// Uses DeviceInstance from config. Sends as BVLC unicast to the provided addr.
func (c *Controller) respondIAm(addr net.Addr) error {
	fmt.Println("responding to IAm")

	// Build APDU objects with configured device instance.
	objs := services.IAmObjects(uint32(c.cfg.DeviceInstance), 1024, 0, 0) // maxAPDU=1024, segmentation=0, vendor=0
	apdu := plumbing.NewAPDU(plumbing.UnConfirmedReq, services.ServiceUnconfirmedIAm, objs)

	// BVLC: use unicast for reply to sender.
	bvlc := plumbing.NewBVLC(plumbing.BVLCFuncUnicast)
	// NPDU: minimal NPDU (no extra specifiers)
	npdu := plumbing.NewNPDU(false, false, false, false)

	iam := &services.UnconfirmedIAm{
		BVLC: bvlc,
		NPDU: npdu,
		APDU: apdu,
	}
	iam.SetLength()

	b, err := iam.MarshalBinary()
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return errors.New("bacnet: connection closed")
	}
	_, err = c.conn.WriteTo(b, addr)
	return err
}

// Helper functions to convert between thermostat and BACnet values

// modeToBACnet converts thermostat.Mode to BACnet multi-state value (1-4)
func modeToBACnet(mode thermostat.Mode) uint32 {
	switch mode {
	case thermostat.ModeHeat:
		return 1
	case thermostat.ModeCool:
		return 2
	case thermostat.ModeFan:
		return 3
	case thermostat.ModeAuto:
		return 4
	default:
		return 0 // unknown
	}
}

// modeFromBACnet converts BACnet multi-state value to thermostat.Mode
func modeFromBACnet(value uint32) (thermostat.Mode, error) {
	switch value {
	case 1:
		return thermostat.ModeHeat, nil
	case 2:
		return thermostat.ModeCool, nil
	case 3:
		return thermostat.ModeFan, nil
	case 4:
		return thermostat.ModeAuto, nil
	default:
		return thermostat.ModeUnknown, fmt.Errorf("invalid mode value: %d", value)
	}
}

// fanSpeedToBACnet converts thermostat.FanSpeed to BACnet multi-state value (1-4)
func fanSpeedToBACnet(fan thermostat.FanSpeed) uint32 {
	switch fan {
	case thermostat.FanAuto:
		return 1
	case thermostat.FanLow:
		return 2
	case thermostat.FanMedium:
		return 3
	case thermostat.FanHigh:
		return 4
	default:
		return 0 // unknown
	}
}

// fanSpeedFromBACnet converts BACnet multi-state value to thermostat.FanSpeed
func fanSpeedFromBACnet(value uint32) (thermostat.FanSpeed, error) {
	switch value {
	case 1:
		return thermostat.FanAuto, nil
	case 2:
		return thermostat.FanLow, nil
	case 3:
		return thermostat.FanMedium, nil
	case 4:
		return thermostat.FanHigh, nil
	default:
		return thermostat.FanUnknown, fmt.Errorf("invalid fan speed value: %d", value)
	}
}

// respondReadProperty handles a Confirmed ReadProperty request
func (c *Controller) respondReadProperty(addr net.Addr, req *services.ConfirmedReadProperty) error {
	dec, err := req.Decode()
	if err != nil {
		return fmt.Errorf("decode read property: %w", err)
	}

	log.Printf("bacnet: ReadProperty request - objectType=%d, instance=%d, property=%d",
		dec.ObjectType, dec.InstanceId, dec.PropertyId)

	// Only handle PresentValue property reads
	if dec.PropertyId != propPresentValue {
		return c.respondError(addr, req, objects.ErrorClassObject, objects.ErrorCodeUnknownObject)
	}

	snapshot := c.svc.Get()
	var value any
	var errResponse error

	switch dec.ObjectType {
	case objTypeAnalogInput:
		// Analog Input - ambient temperature
		if dec.InstanceId == instAnalogInputAmbientTemp {
			value = float32(snapshot.AmbientTemperature)
		} else {
			errResponse = c.respondError(addr, req, objects.ErrorClassObject, objects.ErrorCodeUnknownObject)
			return errResponse
		}

	case objTypeAnalogValue:
		// Analog Value - temperature setpoints
		switch dec.InstanceId {
		case instAnalogValueSetpoint:
			value = float32(snapshot.TemperatureSetpoint)
		case instAnalogValueSetpointMin:
			value = float32(snapshot.TemperatureSetpointMin)
		case instAnalogValueSetpointMax:
			value = float32(snapshot.TemperatureSetpointMax)
		default:
			errResponse = c.respondError(addr, req, objects.ErrorClassObject, objects.ErrorCodeUnknownObject)
			return errResponse
		}

	case objTypeBinaryInput:
		// Binary Input - enabled state
		if dec.InstanceId == instBinaryInputEnabled {
			// BACnet binary values: 0=inactive, 1=active
			if snapshot.Enabled {
				value = uint32(1)
			} else {
				value = uint32(0)
			}
		} else {
			errResponse = c.respondError(addr, req, objects.ErrorClassObject, objects.ErrorCodeUnknownObject)
			return errResponse
		}

	case objTypeMultiStateValue:
		// Multi-state Value - mode and fan speed
		switch dec.InstanceId {
		case instMultiStateValueMode:
			value = modeToBACnet(snapshot.Mode)
		case instMultiStateValueFanSpeed:
			value = fanSpeedToBACnet(snapshot.FanSpeed)
		default:
			errResponse = c.respondError(addr, req, objects.ErrorClassObject, objects.ErrorCodeUnknownObject)
			return errResponse
		}

	default:
		errResponse = c.respondError(addr, req, objects.ErrorClassObject, objects.ErrorCodeUnknownObject)
		return errResponse
	}

	// Build ComplexACK response
	objs := services.ComplexACKObjects(dec.ObjectType, dec.InstanceId, dec.PropertyId, value.(float32))
	apdu := plumbing.NewAPDU(plumbing.ConfirmedReq, services.ServiceConfirmedAcknowledgeAlarm, objs)

	bvlc := plumbing.NewBVLC(plumbing.BVLCFuncUnicast)
	npdu := plumbing.NewNPDU(false, false, false, false)

	ack := &services.ComplexACK{
		BVLC: bvlc,
		NPDU: npdu,
		APDU: apdu,
	}
	ack.SetLength()

	b, err := ack.MarshalBinary()
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return errors.New("bacnet: connection closed")
	}
	_, err = c.conn.WriteTo(b, addr)
	return err
}

// respondWriteProperty handles a Confirmed WriteProperty request
func (c *Controller) respondWriteProperty(addr net.Addr, req *services.ConfirmedWriteProperty) error {
	dec, err := req.Decode()
	if err != nil {
		return fmt.Errorf("decode write property: %w", err)
	}

	log.Printf("bacnet: WriteProperty request - objectType=%d, instance=%d, property=%d",
		dec.ObjectType, dec.InstanceId, dec.PropertyId)

	// Only handle PresentValue property writes
	if dec.PropertyId != propPresentValue {
		return c.respondError(addr, req, objects.ErrorClassObject, objects.ErrorCodeUnknownObject)
	}

	switch dec.ObjectType {
	case objTypeAnalogValue:
		// Analog Value - temperature setpoints
		switch dec.InstanceId {
		case instAnalogValueSetpoint:
			if err := c.svc.SetSetpoint(float64(dec.Value)); err != nil {
				return c.respondError(addr, req, objects.ErrorClassObject, objects.ErrorCodeServiceRequestDenied)
			}

		case instAnalogValueSetpointMin:
			cur := c.svc.Get()
			if err := c.svc.SetMinMax(float64(dec.Value), cur.TemperatureSetpointMax); err != nil {
				return c.respondError(addr, req, objects.ErrorClassObject, objects.ErrorCodeServiceRequestDenied)
			}

		case instAnalogValueSetpointMax:
			cur := c.svc.Get()
			if err := c.svc.SetMinMax(cur.TemperatureSetpointMin, float64(dec.Value)); err != nil {
				return c.respondError(addr, req, objects.ErrorClassObject, objects.ErrorCodeServiceRequestDenied)
			}

		default:
			return c.respondError(addr, req, objects.ErrorClassObject, objects.ErrorCodeUnknownObject)
		}

	case objTypeBinaryInput:
		// Binary Input - enabled state
		if dec.InstanceId == instBinaryInputEnabled {
			// BACnet binary values: 0=inactive, 1=active
			c.svc.SetEnabled(dec.Value != 0)
		} else {
			return c.respondError(addr, req, objects.ErrorClassObject, objects.ErrorCodeUnknownObject)
		}

	case objTypeMultiStateValue:
		// Multi-state Value - mode and fan speed
		switch dec.InstanceId {
		case instMultiStateValueMode:
			mode, err := modeFromBACnet(uint32(dec.Value))
			if err != nil {
				return c.respondError(addr, req, objects.ErrorClassObject, objects.ErrorCodeUnknownObject)
			}
			if err := c.svc.SetMode(mode); err != nil {
				return c.respondError(addr, req, objects.ErrorClassObject, objects.ErrorCodeServiceRequestDenied)
			}

		case instMultiStateValueFanSpeed:
			fanSpeed, err := fanSpeedFromBACnet(uint32(dec.Value))
			if err != nil {
				return c.respondError(addr, req, objects.ErrorClassObject, objects.ErrorCodeUnknownObject)
			}
			if err := c.svc.SetFanSpeed(fanSpeed); err != nil {
				return c.respondError(addr, req, objects.ErrorClassObject, objects.ErrorCodeServiceRequestDenied)
			}

		default:
			return c.respondError(addr, req, objects.ErrorClassObject, objects.ErrorCodeUnknownObject)
		}

	default:
		return c.respondError(addr, req, objects.ErrorClassObject, objects.ErrorCodeUnknownObject)
	}

	// Send SimpleACK response for successful write
	return c.respondSimpleACK(addr, req)
}

// respondSimpleACK sends a SimpleACK response
func (c *Controller) respondSimpleACK(addr net.Addr, req *services.ConfirmedWriteProperty) error {
	apdu := plumbing.NewAPDU(plumbing.ConfirmedReq, services.ServiceConfirmedAcknowledgeAlarm, nil)

	bvlc := plumbing.NewBVLC(plumbing.BVLCFuncUnicast)
	npdu := plumbing.NewNPDU(false, false, false, false)

	ack := &services.ComplexACK{
		BVLC: bvlc,
		NPDU: npdu,
		APDU: apdu,
	}
	ack.SetLength()

	b, err := ack.MarshalBinary()
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return errors.New("bacnet: connection closed")
	}
	_, err = c.conn.WriteTo(b, addr)
	return err
}

// respondError sends an Error response
func (c *Controller) respondError(addr net.Addr, req interface{}, errorClass, errorCode uint8) error {
	objs := services.ErrorObjects(errorClass, errorCode)
	apdu := plumbing.NewAPDU(plumbing.ConfirmedReq, 0, objs) // 0 = Error service

	bvlc := plumbing.NewBVLC(plumbing.BVLCFuncUnicast)
	npdu := plumbing.NewNPDU(false, false, false, false)

	errorMsg := &services.Error{
		BVLC: bvlc,
		NPDU: npdu,
		APDU: apdu,
	}
	errorMsg.SetLength()

	b, err := errorMsg.MarshalBinary()
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return errors.New("bacnet: connection closed")
	}
	_, err = c.conn.WriteTo(b, addr)
	return err
}

// LocalAddr returns the controller's bound address (useful for tests).
// Returns nil if not running.
func (c *Controller) LocalAddr() net.Addr {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	return c.conn.LocalAddr()
}
