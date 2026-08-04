package gamespy

import (
	"encoding/binary"
	"net"
	"strconv"
	"strings"
)

// SignedIPString encodes an IPv4 address as GameSpy signed int string.
// console 1 (Wii) = big-endian, console 0 (DS) = little-endian.
func SignedIPString(ip net.IP, console int) string {
	ip4 := ip.To4()
	if ip4 == nil {
		return "0"
	}
	var v uint32
	if console == 1 {
		v = binary.BigEndian.Uint32(ip4)
	} else {
		v = binary.LittleEndian.Uint32(ip4)
	}
	return strconv.FormatInt(int64(int32(v)), 10)
}

// MatchPublicIP returns true if stored (signed int string OR dotted) refers to the same IPv4 as candidate.
// candidate may be dotted ("1.2.3.4") or a signed int string. Tries both endiannesses when comparing signed forms.
func MatchPublicIP(stored, candidate string) bool {
	if stored == candidate {
		return true
	}
	storedForms := ipv4Forms(stored)
	candForms := ipv4Forms(candidate)
	if len(storedForms) == 0 || len(candForms) == 0 {
		return false
	}
	for _, s := range storedForms {
		for _, c := range candForms {
			if s == c {
				return true
			}
		}
	}
	return false
}

// ParseIPv4Bytes parses "a.b.c.d" into [4]byte (zeros on failure).
func ParseIPv4Bytes(dotted string) [4]byte {
	var out [4]byte
	parts := strings.Split(dotted, ".")
	if len(parts) != 4 {
		return out
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 255 {
			return [4]byte{}
		}
		out[i] = byte(n)
	}
	return out
}

func ipv4Forms(s string) [][4]byte {
	if strings.Contains(s, ".") {
		ip := net.ParseIP(s)
		if ip4 := ip.To4(); ip4 != nil {
			var b [4]byte
			copy(b[:], ip4)
			return [][4]byte{b}
		}
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil
	}
	v := uint32(int32(n))
	var le, be [4]byte
	binary.LittleEndian.PutUint32(le[:], v)
	binary.BigEndian.PutUint32(be[:], v)
	return [][4]byte{le, be}
}
