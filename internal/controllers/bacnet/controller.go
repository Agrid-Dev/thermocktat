package bacnetctrl

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math"
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

// BACnet object types not defined in the library.
const (
	ObjectTypeAnalogValue     uint16 = 2
	ObjectTypeBinaryValue     uint16 = 5
	ObjectTypeMultiStateValue uint16 = 19
)

// objectKey identifies a BACnet object by type and instance.
type objectKey struct {
	ObjectType uint16
	Instance   uint32
}

// property describes how a BACnet object maps to a thermostat field.
type property struct {
	read  func(s thermostat.Snapshot) float32
	write func(svc ports.ThermostatService, v float32) error // nil = read-only
}

// objectMap maps BACnet objects to thermostat fields.
// All objects expose PropertyIdPresentValue (85).
var objectMap = map[objectKey]property{
	// AnalogInput 0 — ambient_temperature (read-only)
	{objects.ObjectTypeAnalogInput, 0}: {
		read: func(s thermostat.Snapshot) float32 { return float32(s.AmbientTemperature) },
	},
	// AnalogValue 0 — temperature_setpoint
	{ObjectTypeAnalogValue, 0}: {
		read:  func(s thermostat.Snapshot) float32 { return float32(s.TemperatureSetpoint) },
		write: func(svc ports.ThermostatService, v float32) error { return svc.SetSetpoint(float64(v)) },
	},
	// AnalogValue 1 — temperature_setpoint_min
	{ObjectTypeAnalogValue, 1}: {
		read: func(s thermostat.Snapshot) float32 { return float32(s.TemperatureSetpointMin) },
		write: func(svc ports.ThermostatService, v float32) error {
			cur := svc.Get()
			return svc.SetMinMax(float64(v), cur.TemperatureSetpointMax)
		},
	},
	// AnalogValue 2 — temperature_setpoint_max
	{ObjectTypeAnalogValue, 2}: {
		read: func(s thermostat.Snapshot) float32 { return float32(s.TemperatureSetpointMax) },
		write: func(svc ports.ThermostatService, v float32) error {
			cur := svc.Get()
			return svc.SetMinMax(cur.TemperatureSetpointMin, float64(v))
		},
	},
	// BinaryValue 0 — enabled (1.0 = active, 0.0 = inactive)
	{ObjectTypeBinaryValue, 0}: {
		read: func(s thermostat.Snapshot) float32 {
			if s.Enabled {
				return 1
			}
			return 0
		},
		write: func(svc ports.ThermostatService, v float32) error {
			svc.SetEnabled(v != 0)
			return nil
		},
	},
	// MultiStateValue 0 — mode (1=heat, 2=cool, 3=fan, 4=auto)
	{ObjectTypeMultiStateValue, 0}: {
		read:  func(s thermostat.Snapshot) float32 { return float32(s.Mode) },
		write: func(svc ports.ThermostatService, v float32) error { return svc.SetMode(thermostat.Mode(v)) },
	},
	// MultiStateValue 1 — fan_speed (1=auto, 2=low, 3=medium, 4=high)
	{ObjectTypeMultiStateValue, 1}: {
		read:  func(s thermostat.Snapshot) float32 { return float32(s.FanSpeed) },
		write: func(svc ports.ThermostatService, v float32) error { return svc.SetFanSpeed(thermostat.FanSpeed(v)) },
	},
	// AnalogValue 3 — fault_code (plain integer, transported as float32)
	{ObjectTypeAnalogValue, 3}: {
		read: func(s thermostat.Snapshot) float32 { return float32(s.FaultCode) },
		write: func(svc ports.ThermostatService, v float32) error {
			svc.SetFaultCode(int(v))
			return nil
		},
	},
}

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

// Run starts a BACnet/IP listener and handles Who-Is, ReadProperty, and WriteProperty.
// It blocks until ctx is canceled.
func (c *Controller) Run(ctx context.Context) error {
	ln, err := net.ListenPacket("udp4", c.cfg.Addr)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.conn = ln
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		if c.conn != nil {
			_ = c.conn.Close()
			c.conn = nil
		}
		c.mu.Unlock()
	}()

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 2048)
		for {
			n, addr, err := ln.ReadFrom(buf)
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					log.Printf("bacnet read error: %v", err)
					continue
				}
			}

			data := make([]byte, n)
			copy(data, buf[:n])

			pkt, err := bacnet.Parse(data)
			if err != nil {
				log.Printf("bacnet: parse error from %s: %v", addr.String(), err)
				continue
			}

			switch msg := pkt.(type) {
			case *services.UnconfirmedWhoIs:
				log.Printf("bacnet: Who-Is from %s — replying I-Am (device %d)", addr, c.cfg.DeviceInstance)
				if err := c.respondIAm(addr); err != nil {
					log.Printf("bacnet: I-Am error: %v", err)
				}
			case *services.ConfirmedReadProperty:
				c.handleReadProperty(msg, addr)
			case *services.ConfirmedWriteProperty:
				c.handleWriteProperty(msg, addr)
			default:
				log.Printf("bacnet: unsupported service from %s: %T", addr, pkt)
			}
		}
	}()

	<-ctx.Done()
	_ = ln.Close()
	<-readDone
	return ctx.Err()
}

// readPropertyRequest holds the decoded fields from a ConfirmedReadProperty.
type readPropertyRequest struct {
	ObjectType uint16
	InstanceId uint32
	PropertyId uint8
}

// writePropertyRequest holds the decoded fields from a ConfirmedWriteProperty.
type writePropertyRequest struct {
	ObjectType uint16
	InstanceId uint32
	PropertyId uint8
	Value      float32
}

// decodeReadProperty extracts fields from a ConfirmedReadProperty using correct bit math.
// The upstream library's DecObjectIdentifier has a bit-shift bug (>> 20 instead of >> 22),
// so we decode the raw APDU objects directly.
func decodeReadProperty(msg *services.ConfirmedReadProperty) (readPropertyRequest, error) {
	if len(msg.APDU.Objects) != 2 {
		return readPropertyRequest{}, fmt.Errorf("expected 2 objects, got %d", len(msg.APDU.Objects))
	}
	objType, instN, err := decodeObjectIdentifier(msg.APDU.Objects[0])
	if err != nil {
		return readPropertyRequest{}, err
	}
	propId, err := decodePropertyIdentifier(msg.APDU.Objects[1])
	if err != nil {
		return readPropertyRequest{}, err
	}
	return readPropertyRequest{ObjectType: objType, InstanceId: instN, PropertyId: propId}, nil
}

// decodeWriteProperty extracts fields from a ConfirmedWriteProperty using correct bit math.
// The upstream library's Decode has the same ObjectIdentifier bit-shift bug.
func decodeWriteProperty(msg *services.ConfirmedWriteProperty) (writePropertyRequest, error) {
	// After parsing, the APDU objects (with opening/closing tags stripped) are:
	// [0] ObjectIdentifier, [1] PropertyIdentifier, [2] Real(value), [3] Null, [4] Priority
	if len(msg.APDU.Objects) < 3 {
		return writePropertyRequest{}, fmt.Errorf("expected >=3 objects, got %d", len(msg.APDU.Objects))
	}
	objType, instN, err := decodeObjectIdentifier(msg.APDU.Objects[0])
	if err != nil {
		return writePropertyRequest{}, err
	}
	propId, err := decodePropertyIdentifier(msg.APDU.Objects[1])
	if err != nil {
		return writePropertyRequest{}, err
	}
	value, err := decodeReal(msg.APDU.Objects[2])
	if err != nil {
		return writePropertyRequest{}, err
	}
	return writePropertyRequest{ObjectType: objType, InstanceId: instN, PropertyId: propId, Value: value}, nil
}

// decodeObjectIdentifier correctly decodes a BACnet Object Identifier from raw APDU payload.
func decodeObjectIdentifier(payload objects.APDUPayload) (objType uint16, instN uint32, err error) {
	obj, ok := payload.(*objects.Object)
	if !ok {
		return 0, 0, errors.New("wrong payload type")
	}
	if obj.Length != 4 {
		return 0, 0, fmt.Errorf("object identifier length %d, want 4", obj.Length)
	}
	joined := binary.BigEndian.Uint32(obj.Data)
	objType = uint16(joined >> 22)
	instN = joined & 0x3FFFFF
	return objType, instN, nil
}

// decodePropertyIdentifier extracts property ID from raw APDU payload.
func decodePropertyIdentifier(payload objects.APDUPayload) (uint8, error) {
	obj, ok := payload.(*objects.Object)
	if !ok {
		return 0, errors.New("wrong payload type")
	}
	if obj.Length != 1 {
		return 0, fmt.Errorf("property identifier length %d, want 1", obj.Length)
	}
	return obj.Data[0], nil
}

// decodeReal extracts a float32 from raw APDU payload.
func decodeReal(payload objects.APDUPayload) (float32, error) {
	obj, ok := payload.(*objects.Object)
	if !ok {
		return 0, errors.New("wrong payload type")
	}
	if obj.Length != 4 {
		return 0, fmt.Errorf("real length %d, want 4", obj.Length)
	}
	bits := binary.BigEndian.Uint32(obj.Data)
	return math.Float32frombits(bits), nil
}

// handleReadProperty decodes a ReadProperty request and responds with ComplexACK or Error.
func (c *Controller) handleReadProperty(msg *services.ConfirmedReadProperty, addr net.Addr) {
	dec, err := decodeReadProperty(msg)
	if err != nil {
		log.Printf("bacnet: ReadProperty decode error: %v", err)
		c.sendError(addr, msg.APDU.InvokeID, services.ServiceConfirmedReadProperty,
			objects.ErrorClassService, objects.ErrorCodeServiceRequestDenied)
		return
	}

	if dec.PropertyId != objects.PropertyIdPresentValue {
		c.sendError(addr, msg.APDU.InvokeID, services.ServiceConfirmedReadProperty,
			objects.ErrorClassService, objects.ErrorCodeServiceRequestDenied)
		return
	}

	prop, ok := objectMap[objectKey{dec.ObjectType, dec.InstanceId}]
	if !ok {
		c.sendError(addr, msg.APDU.InvokeID, services.ServiceConfirmedReadProperty,
			objects.ErrorClassObject, objects.ErrorCodeUnknownObject)
		return
	}

	snap := c.svc.Get()
	value := prop.read(snap)

	c.sendComplexACK(addr, msg.APDU.InvokeID, dec.ObjectType, dec.InstanceId, dec.PropertyId, value)
}

// handleWriteProperty decodes a WriteProperty request and responds with SimpleACK or Error.
func (c *Controller) handleWriteProperty(msg *services.ConfirmedWriteProperty, addr net.Addr) {
	dec, err := decodeWriteProperty(msg)
	if err != nil {
		log.Printf("bacnet: WriteProperty decode error: %v", err)
		c.sendError(addr, msg.APDU.InvokeID, services.ServiceConfirmedWriteProperty,
			objects.ErrorClassService, objects.ErrorCodeServiceRequestDenied)
		return
	}

	if dec.PropertyId != objects.PropertyIdPresentValue {
		c.sendError(addr, msg.APDU.InvokeID, services.ServiceConfirmedWriteProperty,
			objects.ErrorClassService, objects.ErrorCodeServiceRequestDenied)
		return
	}

	prop, ok := objectMap[objectKey{dec.ObjectType, dec.InstanceId}]
	if !ok {
		c.sendError(addr, msg.APDU.InvokeID, services.ServiceConfirmedWriteProperty,
			objects.ErrorClassObject, objects.ErrorCodeUnknownObject)
		return
	}

	if prop.write == nil {
		c.sendError(addr, msg.APDU.InvokeID, services.ServiceConfirmedWriteProperty,
			objects.ErrorClassService, objects.ErrorCodeServiceRequestDenied)
		return
	}

	if err := prop.write(c.svc, dec.Value); err != nil {
		log.Printf("bacnet: WriteProperty apply error: %v", err)
		c.sendError(addr, msg.APDU.InvokeID, services.ServiceConfirmedWriteProperty,
			objects.ErrorClassService, objects.ErrorCodeServiceRequestDenied)
		return
	}

	c.sendSimpleACK(addr, msg.APDU.InvokeID, services.ServiceConfirmedWriteProperty)
}

// respondIAm builds and sends an I-Am response.
// We build objects manually because the library's IAmObjects has a bug
// that ignores the device instance parameter.
func (c *Controller) respondIAm(addr net.Addr) error {
	iamObjs := []objects.APDUPayload{
		objects.EncObjectIdentifier(false, objects.TagBACnetObjectIdentifier, objects.ObjectTypeDevice, uint32(c.cfg.DeviceInstance)),
		objects.EncUnsignedInteger16(1024), // maxAPDULengthAccepted
		objects.EncEnumerated(0),           // segmentationSupported (0 = no)
		objects.EncUnsignedInteger16(0),    // vendorID
	}

	apdu := plumbing.NewAPDU(plumbing.UnConfirmedReq, services.ServiceUnconfirmedIAm, iamObjs)
	bvlc := plumbing.NewBVLC(plumbing.BVLCFuncUnicast)
	npdu := plumbing.NewNPDU(false, false, false, false)

	iam := &services.UnconfirmedIAm{BVLC: bvlc, NPDU: npdu, APDU: apdu}
	iam.SetLength()

	b, err := iam.MarshalBinary()
	if err != nil {
		return err
	}
	return c.send(b, addr)
}

// sendComplexACK sends a ReadProperty success response.
func (c *Controller) sendComplexACK(addr net.Addr, invokeID uint8, objType uint16, instN uint32, propID uint8, value float32) {
	objs := services.ComplexACKObjects(objType, instN, propID, value)
	apdu := plumbing.NewAPDU(plumbing.ComplexAck, services.ServiceConfirmedReadProperty, objs)
	apdu.InvokeID = invokeID

	bvlc := plumbing.NewBVLC(plumbing.BVLCFuncUnicast)
	npdu := plumbing.NewNPDU(false, false, false, false)

	cack := &services.ComplexACK{BVLC: bvlc, NPDU: npdu, APDU: apdu}
	cack.SetLength()

	b, err := cack.MarshalBinary()
	if err != nil {
		log.Printf("bacnet: ComplexACK marshal error: %v", err)
		return
	}
	if err := c.send(b, addr); err != nil {
		log.Printf("bacnet: ComplexACK send error: %v", err)
	}
}

// sendSimpleACK sends a WriteProperty success response.
func (c *Controller) sendSimpleACK(addr net.Addr, invokeID uint8, service uint8) {
	apdu := plumbing.NewAPDU(plumbing.SimpleAck, service, nil)
	apdu.InvokeID = invokeID

	bvlc := plumbing.NewBVLC(plumbing.BVLCFuncUnicast)
	npdu := plumbing.NewNPDU(false, false, false, false)

	sack := &services.SimpleACK{BVLC: bvlc, NPDU: npdu, APDU: apdu}
	sack.SetLength()

	b, err := sack.MarshalBinary()
	if err != nil {
		log.Printf("bacnet: SimpleACK marshal error: %v", err)
		return
	}
	if err := c.send(b, addr); err != nil {
		log.Printf("bacnet: SimpleACK send error: %v", err)
	}
}

// sendError sends a BACnet Error response.
func (c *Controller) sendError(addr net.Addr, invokeID uint8, service uint8, errClass, errCode uint8) {
	objs := services.ErrorObjects(errClass, errCode)
	apdu := plumbing.NewAPDU(plumbing.Error, service, objs)
	apdu.InvokeID = invokeID

	bvlc := plumbing.NewBVLC(plumbing.BVLCFuncUnicast)
	npdu := plumbing.NewNPDU(false, false, false, false)

	e := &services.Error{BVLC: bvlc, NPDU: npdu, APDU: apdu}
	e.SetLength()

	b, err := e.MarshalBinary()
	if err != nil {
		log.Printf("bacnet: Error marshal error: %v", err)
		return
	}
	if err := c.send(b, addr); err != nil {
		log.Printf("bacnet: Error send error: %v", err)
	}
}

// send writes bytes to addr on the shared connection.
func (c *Controller) send(b []byte, addr net.Addr) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return errors.New("bacnet: connection closed")
	}
	_, err := c.conn.WriteTo(b, addr)
	return err
}

// LocalAddr returns the controller's bound address (useful for tests).
func (c *Controller) LocalAddr() net.Addr {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	return c.conn.LocalAddr()
}
