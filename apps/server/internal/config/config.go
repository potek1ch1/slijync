package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	AppBaseURL          string
	HTTPAddr            string
	DatabaseURL         string
	SessionCookieSecret string
	S3Endpoint          string
	S3Region            string
	S3Bucket            string
	S3AccessKeyID       string
	S3SecretAccessKey   string
}

func Load() (Config, error) {
	cfg := Config{
		AppBaseURL:          os.Getenv("APP_BASE_URL"),
		HTTPAddr:            os.Getenv("HTTP_ADDR"),
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		SessionCookieSecret: os.Getenv("SESSION_COOKIE_SECRET"),
		S3Endpoint:          os.Getenv("S3_ENDPOINT"),
		S3Region:            os.Getenv("S3_REGION"),
		S3Bucket:            os.Getenv("S3_BUCKET"),
		S3AccessKeyID:       os.Getenv("S3_ACCESS_KEY_ID"),
		S3SecretAccessKey:   os.Getenv("S3_SECRET_ACCESS_KEY"),
	}

	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8080"
	}

	required := map[string]string{
		"APP_BASE_URL":          cfg.AppBaseURL,
		"DATABASE_URL":          cfg.DatabaseURL,
		"SESSION_COOKIE_SECRET": cfg.SessionCookieSecret,
		"S3_ENDPOINT":           cfg.S3Endpoint,
		"S3_REGION":             cfg.S3Region,
		"S3_BUCKET":             cfg.S3Bucket,
		"S3_ACCESS_KEY_ID":      cfg.S3AccessKeyID,
		"S3_SECRET_ACCESS_KEY":  cfg.S3SecretAccessKey,
	}

	var missing []string
	for key, value := range required {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("%w: %s", errors.New("missing required environment variables"), strings.Join(missing, ", "))
	}
	if len(cfg.SessionCookieSecret) < 32 {
		return Config{}, errors.New("SESSION_COOKIE_SECRET must be at least 32 characters")
	}

	return cfg, nil
}
