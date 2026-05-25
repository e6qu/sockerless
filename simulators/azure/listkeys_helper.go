package main

import (
	"crypto/sha256"
	"encoding/base64"
)

// simListKey32 returns a deterministic 44-character base64-encoded
// 32-byte SAS key derived from the resource ID + key kind. Real
// Azure SAS keys (Service Bus, Event Hubs, Redis, Cosmos read-only
// keys) are random 32-byte values base64-encoded to 44 chars. Using
// sha256 makes the value stable per resource so SDK tests can assert
// equality across PUT → listKeys → connect round-trips without
// having to capture the random key out-of-band.
func simListKey32(resourceID, kind string) string {
	sum := sha256.Sum256([]byte("sim-listkey-32|" + resourceID + "|" + kind))
	return base64.StdEncoding.EncodeToString(sum[:])
}
