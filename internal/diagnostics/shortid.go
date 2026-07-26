package diagnostics

import (
	"crypto/rand"
	"errors"
	"strings"
)

const (
	shortIDPrefix        = "PRAIRIE-"
	legacyShortIDPrefix  = "SILO-"
	shortIDPayloadLength = 12
	shortIDChars         = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
)

var ErrInvalidShortID = errors.New("invalid diagnostics short id")

func NewShortID() (string, error) {
	var random [shortIDPayloadLength]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	var out [shortIDPayloadLength]byte
	for i, b := range random {
		out[i] = shortIDChars[int(b)&31]
	}
	return shortIDPrefix + string(out[:]), nil
}

func ParseShortID(raw string) (string, error) {
	id := strings.ToUpper(strings.TrimSpace(raw))
	prefix := shortIDPrefix
	payload := id
	switch {
	case strings.HasPrefix(id, shortIDPrefix):
		payload = strings.TrimPrefix(id, shortIDPrefix)
	case strings.HasPrefix(id, legacyShortIDPrefix):
		payload = strings.TrimPrefix(id, legacyShortIDPrefix)
		// Preserve legacy SILO- IDs so existing rows remain addressable.
		prefix = legacyShortIDPrefix
	}
	if len(payload) != shortIDPayloadLength {
		return "", ErrInvalidShortID
	}
	for i := 0; i < len(payload); i++ {
		if !isShortIDChar(payload[i]) {
			return "", ErrInvalidShortID
		}
	}
	return prefix + payload, nil
}

func isShortIDChar(ch byte) bool {
	for i := 0; i < len(shortIDChars); i++ {
		if shortIDChars[i] == ch {
			return true
		}
	}
	return false
}
