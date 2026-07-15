// Package domain contains identity invariants without transport or persistence concerns.
package domain

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"
)

type Role string

const (
	RoleBuyer     Role = "buyer"
	RoleOrganizer Role = "organizer"
	RoleAdmin     Role = "admin"
)

var (
	ErrInvalidUser        = errors.New("invalid user")
	ErrEmailTaken         = errors.New("email is already registered")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserDisabled       = errors.New("user is disabled")
)

type User struct {
	ID           string
	Email        string
	DisplayName  string
	Role         Role
	PasswordHash string
	Disabled     bool
}

func NormalizeEmail(value string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(value))
	if len(email) == 0 || len(email) > 254 || strings.ContainsAny(email, "\r\n") {
		return "", fmt.Errorf("%w: email is invalid", ErrInvalidUser)
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || !strings.Contains(email, "@") {
		return "", fmt.Errorf("%w: email is invalid", ErrInvalidUser)
	}
	return email, nil
}

func ValidateDisplayName(value string) (string, error) {
	name := strings.TrimSpace(value)
	length := utf8.RuneCountInString(name)
	if length < 2 || length > 100 || strings.ContainsAny(name, "\r\n\t") {
		return "", fmt.Errorf("%w: display name must contain 2-100 characters", ErrInvalidUser)
	}
	return name, nil
}

func ValidatePassword(value string) error {
	length := len([]byte(value))
	if length < 12 || length > 128 {
		return fmt.Errorf("%w: password must contain 12-128 bytes", ErrInvalidUser)
	}
	return nil
}

func (r Role) Valid() bool { return r == RoleBuyer || r == RoleOrganizer || r == RoleAdmin }
