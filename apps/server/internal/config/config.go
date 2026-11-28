package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type Config struct {
	AppBaseURL             string
	HTTPAddr               string
	DatabaseURL            string
	SessionCookieSecret    string
	S3Endpoint             string
	S3Region               string
	S3Bucket               string
	S3AccessKeyID          string
	S3SecretAccessKey      string
	GoogleOIDCClientID     string
	GoogleOIDCClientSecret string
	OIDCRedirectURL        string
	TLS                    bool
}

func Load() (Config, error) {
	tls, err := strconv.ParseBool(os.Getenv("TLS"))
	if err != nil {
		return Config{}, fmt.Errorf("failed to parse env tls: %w", err)
	}

	cfg := Config{
		AppBaseURL:             os.Getenv("APP_BASE_URL"),
		HTTPAddr:               os.Getenv("HTTP_ADDR"),
		DatabaseURL:            os.Getenv("DATABASE_URL"),
		SessionCookieSecret:    os.Getenv("SESSION_COOKIE_SECRET"),
		S3Endpoint:             os.Getenv("S3_ENDPOINT"),
		S3Region:               os.Getenv("S3_REGION"),
		S3Bucket:               os.Getenv("S3_BUCKET"),
		S3AccessKeyID:          os.Getenv("S3_ACCESS_KEY_ID"),
		S3SecretAccessKey:      os.Getenv("S3_SECRET_ACCESS_KEY"),
		GoogleOIDCClientID:     os.Getenv("GOOGLE_OIDC_CLIENT_ID"),
		GoogleOIDCClientSecret: os.Getenv("GOOGLE_OIDC_CLIENT_SECRET"),
		OIDCRedirectURL:        os.Getenv("GOOGLE_OIDC_REDIRECT_URL"),
		TLS:                    tls,
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

func NewGoogleOAuth2Config(cfg Config) (oauth2.Config, error) {
	var missing []string

	if cfg.GoogleOIDCClientID == "" {
		missing = append(missing, "GOOGLE_OIDC_CLIENT_ID")
	}
	if cfg.GoogleOIDCClientSecret == "" {
		missing = append(missing, "GOOGLE_OIDC_CLIENT_SECRET")
	}
	if cfg.OIDCRedirectURL == "" {
		missing = append(missing, "OIDC_REDIRECT_URL")
	}

	if len(missing) > 0 {
		return oauth2.Config{}, fmt.Errorf("%w: %s", errors.New("missing required environment variables for google auth"), strings.Join(missing, ", "))
	}

	return oauth2.Config{
		ClientID:     cfg.GoogleOIDCClientID,
		ClientSecret: cfg.GoogleOIDCClientSecret,
		RedirectURL:  cfg.OIDCRedirectURL,
		Endpoint:     google.Endpoint,
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}, nil
}

type GoogleOIDC struct {
	OAuth2Config    oauth2.Config
	IDTokenVerifier *oidc.IDTokenVerifier
}

func NewGoogleOIDC(oauth2Config oauth2.Config, provider *oidc.Provider) GoogleOIDC {
	return GoogleOIDC{
		OAuth2Config: oauth2Config,
		IDTokenVerifier: provider.Verifier(&oidc.Config{
			ClientID: oauth2Config.ClientID,
		}),
	}
}
