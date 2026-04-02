package knxctrl

import (
	"encoding/binary"
	"errors"
	"net"
)

// KNXnet/IP header constants.
const (
	headerSize    = 6
	headerVersion = 0x10
	hpaiSize      = 8
	hpaiUDP       = 0x01
)

// KNXnet/IP service type identifiers.
const (
	ServiceConnectRequest          uint16 = 0x0205
	ServiceConnectResponse         uint16 = 0x0206
	ServiceConnectionStateRequest  uint16 = 0x0207
	ServiceConnectionStateResponse uint16 = 0x0208
	ServiceDisconnectRequest       uint16 = 0x0209
	ServiceDisconnectResponse      uint16 = 0x020A
	ServiceTunnelingRequest        uint16 = 0x0420
	ServiceTunnelingACK            uint16 = 0x0421
)

// Connection types for CRI (Connect Request Information).
const (
	ConnTypeTunnel uint8 = 0x04
)

// Header is a parsed KNXnet/IP frame header.
type Header struct {
	ServiceType uint16
	TotalLength uint16
}

func ParseHeader(data []byte) (Header, error) {
	if len(data) < headerSize {
		return Header{}, errors.New("data too short for KNXnet/IP header")
	}
	if data[0] != headerSize {
		return Header{}, errors.New("invalid header length byte")
	}
	if data[1] != headerVersion {
		return Header{}, errors.New("unsupported KNXnet/IP version")
	}
	return Header{
		ServiceType: binary.BigEndian.Uint16(data[2:4]),
		TotalLength: binary.BigEndian.Uint16(data[4:6]),
	}, nil
}

func MarshalHeader(serviceType uint16, totalLength int) []byte {
	buf := make([]byte, headerSize)
	buf[0] = headerSize
	buf[1] = headerVersion
	binary.BigEndian.PutUint16(buf[2:4], serviceType)
	binary.BigEndian.PutUint16(buf[4:6], uint16(totalLength))
	return buf
}

// HPAI is a Host Protocol Address Information structure.
type HPAI struct {
	IP   net.IP
	Port uint16
}

// IsNAT returns true if the HPAI is 0.0.0.0:0, meaning the server should use
// the UDP source address instead (xknx default behavior).
func (h HPAI) IsNAT() bool {
	return h.IP.Equal(net.IPv4zero) && h.Port == 0
}

func ParseHPAI(data []byte) (HPAI, error) {
	if len(data) < hpaiSize {
		return HPAI{}, errors.New("data too short for HPAI")
	}
	if data[0] != hpaiSize {
		return HPAI{}, errors.New("invalid HPAI length")
	}
	if data[1] != hpaiUDP {
		return HPAI{}, errors.New("unsupported HPAI protocol (expected UDP)")
	}
	return HPAI{
		IP:   net.IPv4(data[2], data[3], data[4], data[5]),
		Port: binary.BigEndian.Uint16(data[6:8]),
	}, nil
}

func MarshalHPAI(ip net.IP, port uint16) []byte {
	buf := make([]byte, hpaiSize)
	buf[0] = hpaiSize
	buf[1] = hpaiUDP
	ip4 := ip.To4()
	if ip4 == nil {
		ip4 = net.IPv4zero.To4()
	}
	copy(buf[2:6], ip4)
	binary.BigEndian.PutUint16(buf[6:8], port)
	return buf
}

// MarshalHPAINAT returns an HPAI with 0.0.0.0:0 (NAT mode).
func MarshalHPAINAT() []byte {
	return MarshalHPAI(net.IPv4zero, 0)
}
