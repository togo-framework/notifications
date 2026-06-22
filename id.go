package notifications

import (
	"crypto/rand"
	"encoding/hex"
)

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
