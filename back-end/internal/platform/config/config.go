// Package config provides strict environment-based service configuration.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Env returns a trimmed environment value or its default.
func Env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

// Required returns a non-empty environment value.
func Required(key string) (string, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return "", fmt.Errorf("required environment variable %s is empty", key)
	}
	return value, nil
}

// Duration parses a positive duration from the environment.
func Duration(key string, fallback time.Duration) (time.Duration, error) {
	raw := Env(key, fallback.String())
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return value, nil
}

// Int parses an integer in an inclusive range from the environment.
func Int(key string, fallback, minimum, maximum int) (int, error) {
	raw := Env(key, strconv.Itoa(fallback))
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	if value < minimum || value > maximum {
		return 0, fmt.Errorf("%s must be between %d and %d", key, minimum, maximum)
	}
	return value, nil
}

// Secret requires a non-placeholder secret of at least minBytes bytes.
func Secret(key string, minBytes int) ([]byte, error) {
	value, err := Required(key)
	if err != nil {
		return nil, err
	}
	if len(value) < minBytes {
		return nil, fmt.Errorf("%s must contain at least %d bytes", key, minBytes)
	}
	if strings.Contains(strings.ToLower(value), "change-me") {
		return nil, errors.New(key + " contains a placeholder value")
	}
	return []byte(value), nil
}
