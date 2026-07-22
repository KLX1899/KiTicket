// Package password implements bounded Argon2id password hashing and verification.
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	memoryKiB   uint32 = 64 * 1024
	iterations  uint32 = 3
	parallelism uint8  = 2
	saltLength         = 16
	keyLength   uint32 = 32
)

type Argon2id struct{}

func (Argon2id) Hash(value string) (string, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	digest := argon2.IDKey([]byte(value), salt, iterations, memoryKiB, parallelism, keyLength)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, memoryKiB, iterations, parallelism,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(digest)), nil
}

func (Argon2id) Verify(value, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false, errors.New("stored password hash has an invalid format")
	}
	parameters := strings.Split(parts[3], ",")
	if len(parameters) != 3 {
		return false, errors.New("stored password hash has invalid parameters")
	}
	memory, err := parseParameter(parameters[0], "m=")
	if err != nil || memory < 8*1024 || memory > 128*1024 {
		return false, errors.New("stored password hash has invalid memory cost")
	}
	timeCost, err := parseParameter(parameters[1], "t=")
	if err != nil || timeCost < 1 || timeCost > 10 {
		return false, errors.New("stored password hash has invalid time cost")
	}
	threads, err := parseParameter(parameters[2], "p=")
	if err != nil || threads < 1 || threads > 8 {
		return false, errors.New("stored password hash has invalid parallelism")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 16 || len(salt) > 64 {
		return false, errors.New("stored password hash has invalid salt")
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) < 16 || len(expected) > 64 {
		return false, errors.New("stored password hash has invalid digest")
	}
	actual := argon2.IDKey([]byte(value), salt, uint32(timeCost), uint32(memory), uint8(threads), uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

func parseParameter(raw, prefix string) (uint64, error) {
	if !strings.HasPrefix(raw, prefix) {
		return 0, errors.New("invalid parameter")
	}
	return strconv.ParseUint(strings.TrimPrefix(raw, prefix), 10, 32)
}
