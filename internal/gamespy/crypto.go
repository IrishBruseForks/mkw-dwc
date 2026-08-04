package gamespy

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"time"
)

const spaces48 = "                                                " // 48 spaces

// RC4Encrypt applies GameSpy's modified RC4 to data using key.
func RC4Encrypt(key string, data []byte) []byte {
	keyBytes := []byte(key)
	if len(keyBytes) == 0 {
		return nil
	}

	out := make([]byte, len(data))
	copy(out, data)

	S := make([]byte, 256)
	for i := range S {
		S[i] = byte(i)
	}

	j := 0
	for i := 0; i < 256; i++ {
		j = (j + int(S[i]) + int(keyBytes[i%len(keyBytes)])) & 0xff
		S[i], S[j] = S[j], S[i]
	}

	i := 0
	j = 0
	for x, val := range out {
		i = (i + 1 + int(val)) & 0xff
		j = (j + int(S[i])) & 0xff
		S[i], S[j] = S[j], S[i]
		out[x] ^= S[(int(S[i])+int(S[j]))&0xff]
	}

	return out
}

// PrepareRC4Base64 RC4-encrypts ASCII data, appends a null byte, and standard base64 encodes.
func PrepareRC4Base64(key, data string) string {
	encrypted := RC4Encrypt(key, []byte(data))
	if encrypted == nil {
		encrypted = []byte{}
	}
	encrypted = append(encrypted, 0)
	return base64.StdEncoding.EncodeToString(encrypted)
}

// GenerateResponse builds the MD5 challenge response for profile login.
func GenerateResponse(serverChallenge, acChallenge, clientChallenge, authtoken string) string {
	h := md5.Sum([]byte(acChallenge))
	first := hex.EncodeToString(h[:])

	output := first
	output += spaces48
	output += authtoken
	output += clientChallenge
	output += serverChallenge
	output += first

	h2 := md5.Sum([]byte(output))
	return hex.EncodeToString(h2[:])
}

// GenerateProof builds the MD5 challenge proof (challenge and client challenge swapped vs response).
func GenerateProof(serverChallenge, acChallenge, clientChallenge, authtoken string) string {
	h := md5.Sum([]byte(acChallenge))
	first := hex.EncodeToString(h[:])

	output := first
	output += spaces48
	output += authtoken
	output += serverChallenge
	output += clientChallenge
	output += first

	h2 := md5.Sum([]byte(output))
	return hex.EncodeToString(h2[:])
}

// EncTypeXEncrypt encrypts data using GameSpy EncTypeX (Luigi Auriemma enctypex_decoder.c).
func EncTypeXEncrypt(key, validate string, data []byte) []byte {
	if key == "" || validate == "" || len(data) == 0 {
		return nil
	}

	keyBytes := []byte(key)
	validateBytes := []byte(validate)

	tmpLen := 20
	buf := make([]byte, tmpLen+len(data))
	copy(buf[tmpLen:], data)

	keyLen := len(keyBytes)
	valLen := len(validateBytes)
	rnd := int(^time.Now().Unix())

	for i := 0; i < tmpLen; i++ {
		rnd = int(int32(rnd*0x343FD + 0x269EC3))
		buf[i] = byte(rnd ^ int(keyBytes[i%keyLen]) ^ int(validateBytes[i%valLen]))
	}

	headerLen := 7
	buf[0] = byte((headerLen - 2) ^ 0xec)
	buf[1] = 0x00
	buf[2] = 0x00
	buf[headerLen-1] = byte((tmpLen - headerLen) ^ 0xea)

	header := make([]byte, tmpLen)
	copy(header, buf[:tmpLen])

	encxkey := make([]byte, 261)
	body := enctypexInit(encxkey, keyBytes, validateBytes, buf)
	if body == nil {
		return header
	}

	enctypexFunc6e(encxkey, body)
	return append(header, body...)
}

func enctypexInit(encxkey, key, validate, data []byte) []byte {
	dataLen := len(data)
	if dataLen < 1 {
		return nil
	}

	headerLen := int(data[0]^0xec) + 2
	if dataLen < headerLen {
		return nil
	}

	dataStart := int(data[headerLen-1] ^ 0xea)
	if dataLen < headerLen+dataStart {
		return nil
	}

	body := enctypexFuncx(encxkey, key, validate, data[headerLen:], dataStart)
	if body == nil {
		return nil
	}
	return body[dataStart:]
}

func enctypexFuncx(encxkey, key, validate, data []byte, datalen int) []byte {
	keyLen := len(key)
	valCopy := make([]byte, len(validate))
	copy(valCopy, validate)

	for i := 0; i < datalen; i++ {
		idx := (int(key[i%keyLen]) * i) & 7
		valCopy[idx] ^= valCopy[i&7] ^ data[i]
	}

	enctypexFunc4(encxkey, valCopy, 8)
	return data
}

func enctypexFunc4(encxkey, id []byte, idLen int) {
	if idLen < 1 {
		return
	}

	for i := 0; i < 256; i++ {
		encxkey[i] = byte(i)
	}

	n1 := 0
	n2 := 0
	for i := 255; i >= 0; i-- {
		t1, newN1, newN2 := enctypexFunc5(encxkey, i, id, idLen, n1, n2)
		n1, n2 = newN1, newN2
		t2 := encxkey[i]
		encxkey[i] = encxkey[t1]
		encxkey[t1] = t2
	}

	encxkey[256] = encxkey[1]
	encxkey[257] = encxkey[3]
	encxkey[258] = encxkey[5]
	encxkey[259] = encxkey[7]
	encxkey[260] = encxkey[n1&0xff]
}

func enctypexFunc5(encxkey []byte, cnt int, id []byte, idLen, n1, n2 int) (int, int, int) {
	if cnt == 0 {
		return 0, n1, n2
	}

	mask := 1
	for mask < cnt {
		mask = (mask << 1) + 1
	}

	i := 0
	tmp := 0
	for {
		n1 = int(encxkey[n1&0xff]) + int(id[n2])
		n2++
		if n2 >= idLen {
			n2 = 0
			n1 += idLen
		}
		tmp = n1 & mask
		i++
		if i > 11 {
			tmp %= cnt
		}
		if tmp <= cnt {
			break
		}
	}

	return tmp, n1, n2
}

func enctypexFunc6e(encxkey, data []byte) {
	for i := range data {
		data[i] = enctypexFunc7e(encxkey, data[i])
	}
}

func enctypexFunc7e(encxkey []byte, d byte) byte {
	a := int(encxkey[256])
	b := int(encxkey[257])
	c := int(encxkey[a])
	encxkey[256] = byte((a + 1) & 0xff)
	encxkey[257] = byte((b + c) & 0xff)

	a = int(encxkey[260])
	b = int(encxkey[257])
	b = int(encxkey[b])
	c = int(encxkey[a])
	encxkey[a] = byte(b)

	a = int(encxkey[259])
	b = int(encxkey[257])
	a = int(encxkey[a])
	encxkey[b] = byte(a)

	a = int(encxkey[256])
	b = int(encxkey[259])
	a = int(encxkey[a])
	encxkey[b] = byte(a)

	a = int(encxkey[256])
	encxkey[a] = byte(c)

	b = int(encxkey[258])
	a = int(encxkey[c])
	c = int(encxkey[259])
	b = (a + b) & 0xff
	encxkey[258] = byte(b)

	a = b
	c = int(encxkey[c])
	b = int(encxkey[257])
	b = int(encxkey[b])
	a = int(encxkey[a])
	c = (b + c) & 0xff
	b = int(encxkey[260])
	b = int(encxkey[b])
	c = (b + c) & 0xff
	b = int(encxkey[c])
	c = int(encxkey[256])
	c = int(encxkey[c])
	a = (a + c) & 0xff
	c = int(encxkey[b])
	b = int(encxkey[a])
	result := byte(c ^ b ^ int(d))
	encxkey[260] = result
	encxkey[259] = d

	return result
}
