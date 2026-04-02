package knxctrl

import (
	"net"
	"testing"
)

func TestParseHeader_Valid(t *testing.T) {
	data := []byte{0x06, 0x10, 0x02, 0x05, 0x00, 0x1A}
	h, err := ParseHeader(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.ServiceType != ServiceConnectRequest {
		t.Fatalf("service type: got 0x%04X, want 0x0205", h.ServiceType)
	}
	if h.TotalLength != 26 {
		t.Fatalf("total length: got %d, want 26", h.TotalLength)
	}
}

func TestParseHeader_TooShort(t *testing.T) {
	_, err := ParseHeader([]byte{0x06, 0x10})
	if err == nil {
		t.Fatal("expected error for short data")
	}
}

func TestParseHeader_InvalidVersion(t *testing.T) {
	data := []byte{0x06, 0x20, 0x02, 0x05, 0x00, 0x1A}
	_, err := ParseHeader(data)
	if err == nil {
		t.Fatal("expected error for invalid version")
	}
}

func TestMarshalHeader_RoundTrip(t *testing.T) {
	buf := MarshalHeader(ServiceTunnelingRequest, 20)
	h, err := ParseHeader(buf)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if h.ServiceType != ServiceTunnelingRequest {
		t.Fatalf("service type mismatch")
	}
	if h.TotalLength != 20 {
		t.Fatalf("total length mismatch: got %d", h.TotalLength)
	}
}

func TestParseHPAI_Valid(t *testing.T) {
	data := []byte{0x08, 0x01, 192, 168, 1, 100, 0x0E, 0x57}
	h, err := ParseHPAI(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.IP.Equal(net.IPv4(192, 168, 1, 100)) {
		t.Fatalf("IP mismatch: got %v", h.IP)
	}
	if h.Port != 3671 {
		t.Fatalf("port mismatch: got %d", h.Port)
	}
	if h.IsNAT() {
		t.Fatal("should not be NAT")
	}
}

func TestParseHPAI_NAT(t *testing.T) {
	data := []byte{0x08, 0x01, 0, 0, 0, 0, 0, 0}
	h, err := ParseHPAI(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !h.IsNAT() {
		t.Fatal("expected NAT HPAI")
	}
}

func TestMarshalHPAI_RoundTrip(t *testing.T) {
	ip := net.IPv4(10, 0, 0, 1)
	buf := MarshalHPAI(ip, 5000)
	h, err := ParseHPAI(buf)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if !h.IP.Equal(ip) {
		t.Fatalf("IP mismatch: got %v", h.IP)
	}
	if h.Port != 5000 {
		t.Fatalf("port mismatch: got %d", h.Port)
	}
}
