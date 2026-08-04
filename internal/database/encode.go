package database

import (
	"crypto/rand"
	"encoding/base64"
	"math/big"
	"strings"
)

const Base32Alpha = "0123456789abcdefghijklmnopqrstuv"

// RandomDecimalString returns a string of size decimal digits.
func RandomDecimalString(size int) (string, error) {
	var b strings.Builder
	b.Grow(size)
	for i := 0; i < size; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		b.WriteByte(byte('0' + n.Int64()))
	}
	return b.String(), nil
}

// Base32Encode encodes a positive integer using the GameSpy-style alphabet.
func Base32Encode(num int64) string {
	if num <= 0 {
		return ""
	}

	encoded := make([]byte, 0, 16)
	for num > 0 {
		encoded = append(encoded, Base32Alpha[num&0x1f])
		num >>= 5
	}

	for i, j := 0, len(encoded)-1; i < j; i, j = i+1, j-1 {
		encoded[i], encoded[j] = encoded[j], encoded[i]
	}
	return string(encoded)
}

// GamespyBase64Encode encodes bytes with GameSpy's base64 alphabet variants.
func GamespyBase64Encode(input []byte) string {
	s := base64.StdEncoding.EncodeToString(input)
	s = strings.ReplaceAll(s, "+", "[")
	s = strings.ReplaceAll(s, "/", "]")
	s = strings.ReplaceAll(s, "=", "_")
	return s
}

// BanGameID strips the trailing region character from a gamecd for ban lookups.
func BanGameID(gamecd string) string {
	if len(gamecd) > 0 {
		return gamecd[:len(gamecd)-1]
	}
	return gamecd
}
