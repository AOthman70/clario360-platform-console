package config

import (
	"fmt"
	"os"
	"time"
)

// Config holds runtime configuration, loaded from the environment.
type Config struct {
	// HTTPAddr is the listen address for the console API, e.g. ":8080".
	HTTPAddr string

	// DatabaseURL is the pgx-compatible Postgres DSN backing tenant, licence
	// and audit reads.
	DatabaseURL string

	// JWTPublicKeyPEM is the PEM-encoded RSA public key used to verify RS256
	// tokens minted by the gateway (Slide 5).
	JWTPublicKeyPEM []byte

	// JWTIssuer and JWTAudience, when set, are enforced on every token.
	JWTIssuer   string
	JWTAudience string

	// ReadTimeout / WriteTimeout / ShutdownTimeout bound request and shutdown
	// lifecycles.
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// Load reads configuration from the environment, applying defaults and
// validating required fields. Before reading, it seeds any missing variables
// from a .env file (path overridable via PLATFORM_ENV_FILE, default ".env") —
// values already set in the OS environment always take precedence.
func Load() (*Config, error) {
	envPath := os.Getenv("PLATFORM_ENV_FILE")
	if envPath == "" {
		envPath = ".env"
	}
	if err := loadDotEnv(envPath); err != nil {
		return nil, err
	}

	cfg := &Config{
		HTTPAddr:        env("PLATFORM_HTTP_ADDR", ":8080"),
		DatabaseURL:     os.Getenv("PLATFORM_DATABASE_URL"),
		JWTIssuer:       os.Getenv("PLATFORM_JWT_ISSUER"),
		JWTAudience:     os.Getenv("PLATFORM_JWT_AUDIENCE"),
		ReadTimeout:     15 * time.Second,
		WriteTimeout:    30 * time.Second,
		ShutdownTimeout: 20 * time.Second,
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("config: PLATFORM_DATABASE_URL is required")
	}

	keyPath := os.Getenv("PLATFORM_JWT_PUBLIC_KEY_FILE")
	if keyPath == "" {
		return nil, fmt.Errorf("config: PLATFORM_JWT_PUBLIC_KEY_FILE is required")
	}
	pem, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("config: read JWT public key %q: %w", keyPath, err)
	}
	cfg.JWTPublicKeyPEM = pem

	return cfg, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
