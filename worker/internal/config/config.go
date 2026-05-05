package config

import (
	"errors"
	"os"
)

type Config struct {
	RedisAddr    string
	APIURL       string
	WorkerToken  string
	ArtifactsDir string
}

func Load() (*Config, error) {
	c := &Config{
		RedisAddr:    env("FALCON_REDIS_ADDR", "localhost:6379"),
		APIURL:       env("FALCON_API_URL", "http://localhost:8080"),
		WorkerToken:  os.Getenv("FALCON_WORKER_TOKEN"),
		ArtifactsDir: env("FALCON_ARTIFACTS_DIR", "/data/artifacts"),
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
