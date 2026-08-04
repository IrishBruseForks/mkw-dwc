package tests_test

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/IrishBruse/mkw-dwc/internal/gamespy"
)

func TestCryptoMatchesPythonReference(t *testing.T) {
	key := "9r3Rmy"
	data := []byte("testchallenge")
	got := gamespy.RC4Encrypt(key, data)
	wantHex := "013677249f81df4ec68be908c5"
	want, err := hex.DecodeString(wantHex)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("RC4Encrypt got %x want %s", got, wantHex)
	}

	if got := gamespy.PrepareRC4Base64(key, "testchallenge"); got != "ATZ3JJ+B307Gi+kIxQA=" {
		t.Fatalf("PrepareRC4Base64 got %q", got)
	}

	server := "ABCDEFGHIJ"
	ac := "12345678"
	client := "CLIENTCHAL"
	token := "NDS" + strings.Repeat("a", 80)

	resp := gamespy.GenerateResponse(server, ac, client, token)
	proof := gamespy.GenerateProof(server, ac, client, token)
	if len(resp) != 32 || len(proof) != 32 {
		t.Fatalf("hash lengths resp=%d proof=%d", len(resp), len(proof))
	}
	if resp == proof {
		t.Fatal("response and proof must differ")
	}

	payload := []byte("hello world")
	enc := gamespy.EncTypeXEncrypt(key, "validate123", payload)
	if enc == nil || len(enc) != 20+len(payload) {
		t.Fatalf("EncTypeXEncrypt len=%d", len(enc))
	}
}
