package bleephub

import (
	"crypto/rand"
	"encoding/hex"
)

func mustRandomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand: " + err.Error())
	}
	return b
}

func mustRandomHex(n int) string {
	return hex.EncodeToString(mustRandomBytes(n))
}
