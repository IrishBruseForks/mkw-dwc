package gamespy

import (
	"encoding/binary"
)

// ReadU16LE reads a little-endian uint16 from b.
func ReadU16LE(b []byte) uint16 {
	return binary.LittleEndian.Uint16(b)
}

// ReadU16BE reads a big-endian uint16 from b.
func ReadU16BE(b []byte) uint16 {
	return binary.BigEndian.Uint16(b)
}

// WriteU16LE writes val as little-endian uint16 into b.
func WriteU16LE(b []byte, val uint16) {
	binary.LittleEndian.PutUint16(b, val)
}

// WriteU16BE writes val as big-endian uint16 into b.
func WriteU16BE(b []byte, val uint16) {
	binary.BigEndian.PutUint16(b, val)
}

// ReadCString reads a NUL-terminated string from b starting at off.
// It returns the string (without the NUL) and nextOffset, the index immediately after the NUL.
// If no NUL terminator is found, it returns ("", -1).
func ReadCString(b []byte, off int) (string, int) {
	if off < 0 || off > len(b) {
		return "", -1
	}
	start := off
	for off < len(b) && b[off] != 0 {
		off++
	}
	if off >= len(b) {
		return "", -1
	}
	return string(b[start:off]), off + 1
}

// GetBytesFromIntSigned encodes a signed 32-bit integer.
// console 1 (Wii) uses big-endian, console 0 (DS) uses little-endian.
func GetBytesFromIntSigned(val int64, console int) []byte {
	b := make([]byte, 4)
	if console == 1 {
		binary.BigEndian.PutUint32(b, uint32(int32(val)))
	} else {
		binary.LittleEndian.PutUint32(b, uint32(int32(val)))
	}
	return b
}
