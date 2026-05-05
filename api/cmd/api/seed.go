package main

import (
	"context"
	"database/sql"
	"errors"

	"github.com/erhan/falcon/api/internal/auth"
	"github.com/jmoiron/sqlx"
)

func seedAdmin(ctx context.Context, db *sqlx.DB, email, password string) error {
	if email == "" || password == "" {
		return errors.New("admin email/password required")
	}
	var count int
	if err := db.GetContext(ctx, &count, `SELECT COUNT(*) FROM users WHERE lower(email)=lower($1)`, email); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO users(email,password_hash) VALUES($1,$2)`, email, hash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return nil
}
