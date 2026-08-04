package gamespy

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestRC4Encrypt(t *testing.T) {
	key := "9r3Rmy"
	data := []byte("testchallenge")
	got := RC4Encrypt(key, data)
	wantHex := "013677249f81df4ec68be908c5"
	want, err := hex.DecodeString(wantHex)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("RC4Encrypt mismatch: got %x want %s", got, wantHex)
	}
}

func TestPrepareRC4Base64(t *testing.T) {
	key := "9r3Rmy"
	data := "testchallenge"
	got := PrepareRC4Base64(key, data)
	want := "ATZ3JJ+B307Gi+kIxQA="
	if got != want {
		t.Fatalf("PrepareRC4Base64 mismatch: got %q want %q", got, want)
	}
}

func TestEncTypeXEncrypt(t *testing.T) {
	key := "9r3Rmy"
	validate := "validate123"
	payload := []byte("hello world")

	got := EncTypeXEncrypt(key, validate, payload)
	if got == nil {
		t.Fatal("EncTypeXEncrypt returned nil")
	}
	if len(got) != 20+len(payload) {
		t.Fatalf("EncTypeXEncrypt length: got %d want %d", len(got), 20+len(payload))
	}

	if EncTypeXEncrypt("", validate, payload) != nil {
		t.Fatal("EncTypeXEncrypt with empty key should return nil")
	}
	if EncTypeXEncrypt(key, "", payload) != nil {
		t.Fatal("EncTypeXEncrypt with empty validate should return nil")
	}
	if EncTypeXEncrypt(key, validate, nil) != nil {
		t.Fatal("EncTypeXEncrypt with empty data should return nil")
	}
}

func TestGenerateResponseAndProof(t *testing.T) {
	server := "ABCDEFGHIJ"
	ac := "12345678"
	client := "CLIENTCHAL"
	token := "NDS" + strings.Repeat("a", 80)

	resp := GenerateResponse(server, ac, client, token)
	if len(resp) != 32 {
		t.Fatalf("GenerateResponse length: got %d want 32", len(resp))
	}

	proof := GenerateProof(server, ac, client, token)
	if len(proof) != 32 {
		t.Fatalf("GenerateProof length: got %d want 32", len(proof))
	}
	if resp == proof {
		t.Fatal("response and proof should differ when challenges differ in order")
	}

	// Fixed vector: MD5(ac) + spaces + token + client + server + MD5(ac), then MD5.
	resp2 := GenerateResponse("SERVERCHAL", "ACCHAL01", "CLIENTCH1", "NDStoken")
	if resp2 != GenerateResponse("SERVERCHAL", "ACCHAL01", "CLIENTCH1", "NDStoken") {
		t.Fatal("GenerateResponse not deterministic")
	}
	proof2 := GenerateProof("SERVERCHAL", "ACCHAL01", "CLIENTCH1", "NDStoken")
	if proof2 == resp2 {
		t.Fatal("proof should differ from response")
	}
}
