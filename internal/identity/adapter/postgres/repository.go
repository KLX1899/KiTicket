// Package postgres stores identity records in the identity-owned schema.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/KLX1899/KiTicket/internal/identity/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) (*Repository, error) {
	if pool == nil {
		return nil, errors.New("identity repository requires a PostgreSQL pool")
	}
	return &Repository{pool: pool}, nil
}

func (r *Repository) Create(ctx context.Context, user domain.User) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO identity.users (id, email, password_hash, display_name, role)
		VALUES ($1, $2, $3, $4, $5)`, user.ID, user.Email, user.PasswordHash, user.DisplayName, user.Role)
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return domain.ErrEmailTaken
	}
	return fmt.Errorf("create user: %w", err)
}

func (r *Repository) ByEmail(ctx context.Context, email string) (domain.User, error) {
	var user domain.User
	var status string
	err := r.pool.QueryRow(ctx, `
		SELECT id, email, display_name, role, password_hash, status
		FROM identity.users
		WHERE email = $1`, email).Scan(&user.ID, &user.Email, &user.DisplayName, &user.Role, &user.PasswordHash, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrInvalidCredentials
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("find user by email: %w", err)
	}
	user.Disabled = status != "active"
	return user, nil
}
