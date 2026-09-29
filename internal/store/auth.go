package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/local/claude-relay/internal/cryptox"
)

func (s *Store) BootstrapAdmin(ctx context.Context, username, password string) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_users`).Scan(&count); err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}
	hash, err := cryptox.HashPassword(password)
	if err != nil {
		return false, err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO admin_users(username, password_hash, created_at) VALUES(?, ?, ?)`,
		username, hash, time.Now().Unix())
	if err != nil {
		return false, fmt.Errorf("create initial administrator: %w", err)
	}
	return true, nil
}

func (s *Store) AuthenticateAdmin(ctx context.Context, username, password string) (AdminUser, error) {
	var user AdminUser
	var hash string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash FROM admin_users WHERE username = ?`, username).
		Scan(&user.ID, &user.Username, &hash)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !cryptox.VerifyPassword(hash, password)) {
		return AdminUser{}, ErrNotFound
	}
	if err != nil {
		return AdminUser{}, err
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE admin_users SET last_login_at = ? WHERE id = ?`, time.Now().Unix(), user.ID)
	return user, nil
}

func (s *Store) ChangePassword(ctx context.Context, userID int64, current, next string) error {
	var existing string
	if err := s.db.QueryRowContext(ctx, `SELECT password_hash FROM admin_users WHERE id = ?`, userID).Scan(&existing); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if !cryptox.VerifyPassword(existing, current) {
		return ErrNotFound
	}
	hash, err := cryptox.HashPassword(next)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE admin_users SET password_hash = ? WHERE id = ?`, hash, userID)
	return err
}

func (s *Store) CreateSession(ctx context.Context, userID int64, ttl time.Duration) (string, time.Time, error) {
	token, err := cryptox.RandomToken(32)
	if err != nil {
		return "", time.Time{}, err
	}
	now := time.Now()
	expires := now.Add(ttl)
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO sessions(token_hash, user_id, created_at, expires_at) VALUES(?, ?, ?, ?)`,
		cryptox.HashSecret(token), userID, now.Unix(), expires.Unix())
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

func (s *Store) SessionUser(ctx context.Context, token string) (AdminUser, error) {
	var user AdminUser
	err := s.db.QueryRowContext(ctx, `SELECT u.id, u.username
		FROM sessions s JOIN admin_users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires_at > ?`,
		cryptox.HashSecret(token), time.Now().Unix()).Scan(&user.ID, &user.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return AdminUser{}, ErrNotFound
	}
	return user, err
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, cryptox.HashSecret(token))
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, time.Now().Unix())
	return err
}

func (s *Store) DeleteUserSessions(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

func (s *Store) AddAudit(ctx context.Context, userID int64, action, resourceType, resourceID, detail, ip string) {
	_, _ = s.db.ExecContext(ctx, `INSERT INTO audit_logs(
		user_id, action, resource_type, resource_id, detail, ip, created_at
	) VALUES(?, ?, ?, ?, ?, ?, ?)`, userID, action, resourceType, resourceID, detail, ip, time.Now().Unix())
}
