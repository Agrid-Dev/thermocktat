package knxctrl

import "math"

// DPT 9.001 — 2-byte KNX float.
// Encoding: value = 0.01 * mantissa * 2^exponent
// Byte layout: [MEEEEMMM] [MMMMMMMM]
//   E = 4-bit exponent (bits 14..11)
//   M = 11-bit two's complement mantissa (bit 15 = sign, bits 10..0 = magnitude)

func EncodeDPT9(v float64) [2]byte {
	// Find smallest exponent where mantissa fits in [-2048, 2047].
	scaled := v * 100
	var exp int
	for exp = 0; exp <= 15; exp++ {
		m := scaled / math.Pow(2, float64(exp))
		if m >= -2048 && m <= 2047 {
			break
		}
	}
	exp = min(exp, 15)
	mantissa := int(math.Round(scaled / math.Pow(2, float64(exp))))
	// Clamp mantissa to 11-bit two's complement range.
	mantissa = min(mantissa, 2047)
	mantissa = max(mantissa, -2048)

	// Encode: sign in bit 15, exponent in bits 14..11, magnitude in bits 10..0.
	var raw uint16
	if mantissa < 0 {
		raw = uint16(mantissa&0x07FF) | 0x8000
	} else {
		raw = uint16(mantissa & 0x07FF)
	}
	raw |= uint16(exp&0x0F) << 11

	return [2]byte{byte(raw >> 8), byte(raw)}
}

func DecodeDPT9(b [2]byte) float64 {
	raw := uint16(b[0])<<8 | uint16(b[1])
	exp := int((raw >> 11) & 0x0F)
	mantissa := int(raw & 0x07FF)
	if raw&0x8000 != 0 {
		// Negative: sign-extend from 11 bits.
		mantissa |= ^0x07FF // set upper bits
	}
	return 0.01 * float64(mantissa) * math.Pow(2, float64(exp))
}

// DPT 1.001 — 1-bit Switch.
// Compact APCI: the value is in the low 6 bits of the APCI byte.

func EncodeDPT1(v bool) byte {
	if v {
		return 1
	}
	return 0
}

func DecodeDPT1(b byte) bool {
	return b&0x01 != 0
}
