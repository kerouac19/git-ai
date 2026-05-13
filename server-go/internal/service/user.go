package service

import (
	"context"
	"fmt"

	"git-ai-server/internal/model"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type UserService struct {
	Pool *pgxpool.Pool
}

func (s *UserService) FindByUsernameOrEmail(ctx context.Context, login string) (*model.User, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT u.id, u.username, COALESCE(u.email, ''), COALESCE(u.display_name, ''),
		       u.password_hash, u.role, u.status, u.org_id::text, o.name,
		       u.created_at, u.updated_at
		FROM users u
		JOIN orgs o ON o.id = u.org_id
		WHERE u.username = $1 OR u.email = $1
		LIMIT 1`, login)

	return scanUser(row)
}

func (s *UserService) FindByID(ctx context.Context, id string) (*model.User, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT u.id, u.username, COALESCE(u.email, ''), COALESCE(u.display_name, ''),
		       u.password_hash, u.role, u.status, u.org_id::text, o.name,
		       u.created_at, u.updated_at
		FROM users u
		JOIN orgs o ON o.id = u.org_id
		WHERE u.id = $1`, id)

	return scanUser(row)
}

func (s *UserService) Create(ctx context.Context, user *model.User) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO users (username, email, display_name, password_hash, role, status)
		VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), $4, $5, $6)`,
		user.Username, user.Email, user.DisplayName, user.PasswordHash, user.Role, user.Status)
	if err != nil {
		return fmt.Errorf("creating user: %w", err)
	}
	return nil
}

func (s *UserService) UserCount(ctx context.Context) (int, error) {
	var count int
	err := s.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

func HashPassword(plain string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}
	return string(hash), nil
}

func ValidatePassword(hashed, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain))
}

func scanUser(row pgx.Row) (*model.User, error) {
	var u model.User
	err := row.Scan(
		&u.ID, &u.Username, &u.Email, &u.DisplayName,
		&u.PasswordHash, &u.Role, &u.Status, &u.OrgID, &u.OrgName,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanning user: %w", err)
	}
	return &u, nil
}

// GetUserOrg returns the user's org id and name via JOIN. Returns ("", "", nil)
// if the user does not exist (caller decides how to handle missing user — typically
// already gated by JWT auth so this is rare).
func (s *UserService) GetUserOrg(ctx context.Context, userID string) (string, string, error) {
	var orgID, orgName string
	err := s.Pool.QueryRow(ctx, `
		SELECT o.id::text, o.name
		FROM users u
		JOIN orgs o ON o.id = u.org_id
		WHERE u.id = $1`, userID).Scan(&orgID, &orgName)
	if err == pgx.ErrNoRows {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("getting user org: %w", err)
	}
	return orgID, orgName, nil
}
