package character

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// HandItemFingerprint is the v1 suspension fingerprint over the typed item's
// canonical JSON. Consumers use it to verify a provider's bounded transition.
func HandItemFingerprint(item Item) string {
	body, _ := json.Marshal(item)
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}
