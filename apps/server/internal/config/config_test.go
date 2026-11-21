package config

import "testing"

func TestLoadRejectsMissingConfiguration(t *testing.T) {
	t.Setenv("APP_BASE_URL", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("SESSION_COOKIE_SECRET", "")
	t.Setenv("S3_ENDPOINT", "")
	t.Setenv("S3_REGION", "")
	t.Setenv("S3_BUCKET", "")
	t.Setenv("S3_ACCESS_KEY_ID", "")
	t.Setenv("S3_SECRET_ACCESS_KEY", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() expected an error")
	}
}
