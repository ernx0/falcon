package config

import (
	"errors"
	"os"
)

type Config struct {
	ListenAddr     string
	DBDSN          string
	RedisAddr      string
	JWTSecret      string
	WorkerToken    string
	AdminEmail     string
	AdminPassword  string
	ArtifactsDir   string
	MigrationsDir  string
}

func Load() (*Config, error) {
	c := &Config{
		ListenAddr:    env("FALCON_LISTEN_ADDR", ":8080"),
		DBDSN:         os.Getenv("FALCON_DB_DSN"),
		RedisAddr:     env("FALCON_REDIS_ADDR", "localhost:6379"),
		JWTSecret:     os.Getenv("FALCON_JWT_SECRET"),
		WorkerToken:   os.Getenv("FALCON_WORKER_TOKEN"),
		AdminEmail:    env("FALCON_ADMIN_EMAIL", "admin@falcon.local"),
		AdminPassword: env("FALCON_ADMIN_PASSWORD", "admin"),
		ArtifactsDir:  env("FALCON_ARTIFACTS_DIR", "/data/artifacts"),
		MigrationsDir: env("FALCON_MIGRATIONS_DIR", "./migrations"),
	}
	if c.DBDSN == "" {
		return nil, errors.New("FALCON_DB_DSN is required")
	}
	if len(c.JWTSecret) < 16 {
		return nil, errors.New("FALCON_JWT_SECRET must be at least 16 chars")
	}
	if c.WorkerToken == "" {
		return nil, errors.New("FALCON_WORKER_TOKEN is required")
	}
	return c, nil
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
