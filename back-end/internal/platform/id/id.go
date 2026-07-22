// Package id creates cryptographically unpredictable identifiers.
package id

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// New returns a 128-bit random hexadecimal identifier.
func New() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate random identifier: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

// Generator adapts New to application ports.
type Generator struct{}

func (Generator) New() (string, error) { return New() }
