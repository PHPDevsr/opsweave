// Package event defines validated automatic ingress and safe receipts.
package event

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
)

var (
	ErrRoute        = errors.New("event destination unavailable")
	ErrConflict     = errors.New("event ID already used with different content")
	ErrDestinations = errors.New("event exceeds the destination limit")
)

// Request is a validated source identity. Webhook code supplies only the body hash.
type Request struct {
	Type, Name, ID, Workflow, Trigger, Fingerprint string
}

// Receipt identifies an already committed automatic admission.
type Receipt struct {
	Execution string `json:"execution"`
	Workflow  string `json:"workflow"`
	Trigger   string `json:"trigger"`
}

// HistoryEntry records a safe admission outcome without request content or credentials.
type HistoryEntry struct {
	Sequence   int64     `json:"sequence"`
	Revision   string    `json:"revision"`
	Trigger    string    `json:"trigger"`
	Type       string    `json:"type"`
	OccurredAt time.Time `json:"occurredAt"`
	Code       string    `json:"code"`
	Execution  string    `json:"execution,omitempty"`
}

// ValidName validates bounded event names and literal path components.
func ValidName(s string) bool {
	if len(s) < 1 || len(s) > 63 || s[0] < 'a' || s[0] > 'z' || s[len(s)-1] == '-' {
		return false
	}
	for i, b := range []byte(s) {
		if b >= 'a' && b <= 'z' || b >= '0' && b <= '9' {
			continue
		}
		if b != '-' || i == 0 || s[i-1] == '-' {
			return false
		}
	}
	return true
}

// ValidID checks opaque event IDs before hashing or signing them.
func ValidID(s string) bool {
	if len(s) < 1 || len(s) > 128 {
		return false
	}
	for _, b := range []byte(s) {
		if b < 33 || b > 126 {
			return false
		}
	}
	return true
}

// Digest returns the canonical SHA-256 hex representation.
func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ValidDigest validates adapter-owned fingerprints at the repository boundary.
func ValidDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, b := range []byte(s) {
		if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f') {
			return false
		}
	}
	return true
}
