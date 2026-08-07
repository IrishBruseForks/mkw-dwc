package nas

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestDecodeNASValueHyphenToSlash(t *testing.T) {
	// Python qs_to_dict maps '-' -> '/' before base64 decode.
	raw := []byte("test/devname")
	encoded := base64.StdEncoding.EncodeToString(raw)
	encoded = strings.ReplaceAll(encoded, "=", "*")
	encoded = strings.ReplaceAll(encoded, "+", ">")
	encoded = strings.ReplaceAll(encoded, "/", "-")

	got, err := decodeNASValue(encoded)
	if err != nil {
		t.Fatalf("decodeNASValue: %v", err)
	}
	if got != string(raw) {
		t.Fatalf("got %q want %q", got, string(raw))
	}
}
