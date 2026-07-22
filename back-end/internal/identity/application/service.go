// Package application implements identity use cases through infrastructure ports.
package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/KLX1899/KiTicket/internal/identity/domain"
)

type Repository interface {
	Create(context.Context, domain.User) error
	ByEmail(context.Context, string) (domain.User, error)
}

type PasswordHasher interface {
	Hash(string) (string, error)
	Verify(string, string) (bool, error)
}

type TokenIssuer interface {
	Issue(subject, role, tokenID string, ttl time.Duration) (string, time.Time, error)
}

type IDGenerator interface{ New() (string, error) }

type Service struct {
	repository Repository
	hasher     PasswordHasher
	tokens     TokenIssuer
	ids        IDGenerator
	accessTTL  time.Duration
}

type RegisterCommand struct {
	Email       string
	DisplayName string
	Password    string
}

type LoginCommand struct {
	Email    string
	Password string
}

type Session struct {
	AccessToken string     `json:"access_token"`
	TokenType   string     `json:"token_type"`
	ExpiresAt   time.Time  `json:"expires_at"`
	User        PublicUser `json:"user"`
}

type PublicUser struct {
	ID          string      `json:"id"`
	Email       string      `json:"email"`
	DisplayName string      `json:"display_name"`
	Role        domain.Role `json:"role"`
}

func New(repository Repository, hasher PasswordHasher, tokens TokenIssuer, ids IDGenerator, accessTTL time.Duration) (*Service, error) {
	if repository == nil || hasher == nil || tokens == nil || ids == nil || accessTTL <= 0 {
		return nil, errors.New("identity service dependencies and positive access TTL are required")
	}
	return &Service{repository: repository, hasher: hasher, tokens: tokens, ids: ids, accessTTL: accessTTL}, nil
}

func (s *Service) Register(ctx context.Context, command RegisterCommand) (Session, error) {
	email, err := domain.NormalizeEmail(command.Email)
	if err != nil {
		return Session{}, err
	}
	displayName, err := domain.ValidateDisplayName(command.DisplayName)
	if err != nil {
		return Session{}, err
	}
	if err := domain.ValidatePassword(command.Password); err != nil {
		return Session{}, err
	}
	passwordHash, err := s.hasher.Hash(command.Password)
	if err != nil {
		return Session{}, fmt.Errorf("hash password: %w", err)
	}
	userID, err := s.ids.New()
	if err != nil {
		return Session{}, fmt.Errorf("generate user ID: %w", err)
	}
	user := domain.User{ID: userID, Email: email, DisplayName: displayName, Role: domain.RoleBuyer, PasswordHash: passwordHash}
	if err := s.repository.Create(ctx, user); err != nil {
		return Session{}, err
	}
	return s.session(user)
}

func (s *Service) Login(ctx context.Context, command LoginCommand) (Session, error) {
	email, err := domain.NormalizeEmail(command.Email)
	if err != nil || len(command.Password) > 128 {
		return Session{}, domain.ErrInvalidCredentials
	}
	user, err := s.repository.ByEmail(ctx, email)
	if err != nil {
		return Session{}, domain.ErrInvalidCredentials
	}
	valid, err := s.hasher.Verify(command.Password, user.PasswordHash)
	if err != nil {
		return Session{}, fmt.Errorf("verify password: %w", err)
	}
	if !valid {
		return Session{}, domain.ErrInvalidCredentials
	}
	if user.Disabled {
		return Session{}, domain.ErrUserDisabled
	}
	return s.session(user)
}

func (s *Service) session(user domain.User) (Session, error) {
	tokenID, err := s.ids.New()
	if err != nil {
		return Session{}, fmt.Errorf("generate token ID: %w", err)
	}
	token, expiresAt, err := s.tokens.Issue(user.ID, string(user.Role), tokenID, s.accessTTL)
	if err != nil {
		return Session{}, fmt.Errorf("issue access token: %w", err)
	}
	return Session{
		AccessToken: token, TokenType: "Bearer", ExpiresAt: expiresAt,
		User: PublicUser{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName, Role: user.Role},
	}, nil
}
