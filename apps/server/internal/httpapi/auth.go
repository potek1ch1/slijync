package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"uuid"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/potek1ch1/slijync/apps/server/internal/config"
	db "github.com/potek1ch1/slijync/apps/server/sqlc"
	"golang.org/x/oauth2"
)

const (
	StateCookie    = "oidc_state"
	NonceCookie    = "oidc_nonce"
	VerifierCookie = "oidc_verifier"
)

var errEmailAlreadyExists = errors.New(
	"email is already associated with another identity",
)

const LoginSessionDuration = 7 * 24 * time.Hour

type OIDCClaims struct {
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

type oidcCookies struct {
	state    *http.Cookie
	nonce    *http.Cookie
	verifier *http.Cookie
}

type cookieValues struct {
	name     string
	value    string
	path     string
	expires  time.Time
	maxAge   int
	httpOnly bool
	secure   bool
	sameSite http.SameSite
}

func handleRedirect(googleOIDC config.GoogleOIDC, cfg config.Config) http.HandlerFunc {
	randomString := func(size int) (string, error) {
		value := make([]byte, size)

		if _, err := rand.Read(value); err != nil {
			return "", err
		}

		return base64.RawURLEncoding.EncodeToString(value), nil
	}

	return func(w http.ResponseWriter, r *http.Request) {
		state, err := randomString(32)
		if err != nil {
			http.Error(w, "failed to start authentication", http.StatusInternalServerError)
			return
		}

		nonce, err := randomString(32)
		if err != nil {
			http.Error(w, "failed to start authentication", http.StatusInternalServerError)
			return
		}

		verifier := oauth2.GenerateVerifier()
		authURL := googleOIDC.OAuth2Config.AuthCodeURL(
			state,
			oauth2.S256ChallengeOption(verifier),
			oidc.Nonce(nonce),
		)

		setOIDCCookie(w, StateCookie, state, cfg.TLS)
		setOIDCCookie(w, NonceCookie, nonce, cfg.TLS)
		setOIDCCookie(w, VerifierCookie, verifier, cfg.TLS)

		http.Redirect(w, r, authURL, http.StatusFound)
	}
}

func handleCallback(dbPool *pgxpool.Pool, queries *db.Queries, googleOIDC config.GoogleOIDC, logger *slog.Logger, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		requestID := middleware.GetReqID(ctx)
		clearOIDCCookies(w, cfg.TLS)

		if providerError := r.URL.Query().Get("error"); providerError != "" {
			logger.Error("failed to authorize with Google OIDC",
				"request_id", requestID,
				"provider_error", providerError,
			)
			http.Error(w, "authentication was not completed", http.StatusBadRequest)
			return
		}

		code := r.URL.Query().Get("code")
		returnedState := r.URL.Query().Get("state")
		if code == "" || returnedState == "" {
			logger.Error("Google OIDC callback is missing required parameters",
				"request_id", requestID,
			)
			http.Error(w, "invalid authentication request", http.StatusBadRequest)
			return
		}

		oidcCookies, err := getOIDCCookies(r)
		if err != nil {
			logger.Error("failed to read OIDC cookies",
				"request_id", requestID,
				"error", err,
			)
			http.Error(w, "authentication session expired", http.StatusBadRequest)
			return
		}

		if subtle.ConstantTimeCompare([]byte(returnedState), []byte(oidcCookies.state.Value)) != 1 {
			logger.Error("Google OIDC state mismatch", "request_id", requestID, "error", err)
			http.Error(w, "invalid authentication request", http.StatusBadRequest)
			return
		}

		// verify oidc
		idToken, err := verifyOIDC(ctx, googleOIDC, oidcCookies, code)
		if err != nil {
			logger.Error("failed to verify oidc", "request_id", requestID, "error", err)
			http.Error(w, "authentication failed", http.StatusUnauthorized)
			return
		}

		var claims OIDCClaims
		if err := idToken.Claims(&claims); err != nil {
			logger.Error("failed to extract Google OIDC ID token claims",
				"request_id", requestID,
				"error", err,
			)
			http.Error(w, "authentication failed", http.StatusUnauthorized)
			return
		}
		if !claims.EmailVerified {
			logger.Error("Google OIDC email is not verified", "request_id", requestID)
			http.Error(w, "verified email is required", http.StatusForbidden)
			return
		}

		// find or register user info
		user, err := findOrCreateUser(ctx, dbPool, queries, logger, idToken, claims)
		if err != nil {
			logger.Error("failed to find or create user", "request_id", requestID, "error", err)
			if errors.Is(err, errEmailAlreadyExists) {
				http.Error(w, "account already exists", http.StatusConflict)
				return
			}
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		//login session
		cookieValues, err := createLoginSession(ctx, *queries, user)
		if err != nil {
			logger.Error(
				"failed to create login session",
				"request_id", requestID,
				"error", err,
			)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "slijync_session",
			Value:    cookieValues.value,
			Path:     "/",
			Expires:  cookieValues.expires, // MaxAge の指定があると MaxAge が優先される
			MaxAge:   cookieValues.maxAge,
			HttpOnly: true,
			Secure:   cfg.TLS,
			SameSite: http.SameSiteLaxMode,
		})

		http.Redirect(w, r, cfg.AppBaseURL, http.StatusFound)
	}
}

func getOIDCCookies(r *http.Request) (oidcCookies, error) {
	stateCookie, err := r.Cookie(StateCookie)
	if err != nil {
		return oidcCookies{}, fmt.Errorf("read OIDC state cookie: %w", err)
	}
	nonceCookie, err := r.Cookie(NonceCookie)
	if err != nil {
		return oidcCookies{}, fmt.Errorf("read OIDC nonce cookie: %w", err)
	}

	verifierCookie, err := r.Cookie(VerifierCookie)
	if err != nil {
		return oidcCookies{}, fmt.Errorf("read OIDC verifier cookie: %w", err)
	}

	return oidcCookies{state: stateCookie, nonce: nonceCookie, verifier: verifierCookie}, nil
}

func clearOIDCCookies(w http.ResponseWriter, secure bool) {
	for _, name := range []string{StateCookie, NonceCookie, VerifierCookie} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/api/auth/google/callback",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		})
	}
}

func setOIDCCookie(w http.ResponseWriter, name, value string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/api/auth/google/callback",
		MaxAge:   300,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func generateLoginSessionToken() (string, [32]byte, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", [32]byte{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	tokenHash := hashLoginSessionToken(token)
	return token, tokenHash, nil
}

func hashLoginSessionToken(token string) [32]byte {
	return sha256.Sum256([]byte(token))
}

func createOIDCUser(dbPool *pgxpool.Pool, queries *db.Queries, ctx context.Context, idToken *oidc.IDToken, claims OIDCClaims) (db.User, error) {
	tx, err := dbPool.Begin(ctx)
	if err != nil {
		return db.User{}, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	txQueries := queries.WithTx(tx)

	user, err := txQueries.CreateUser(ctx, db.CreateUserParams{
		ID: pgtype.UUID{
			Bytes: uuid.NewV7(),
			Valid: true,
		},
		Email:           claims.Email,
		DisplayName:     claims.Name,
		NormalizedEmail: strings.ToLower(strings.TrimSpace(claims.Email)),
	})
	if err != nil {
		return db.User{}, fmt.Errorf("failed to create user: %w", err)
	}

	_, err = txQueries.CreateOIDCIdentity(ctx, db.CreateOIDCIdentityParams{
		ID: pgtype.UUID{
			Bytes: uuid.NewV7(),
			Valid: true,
		},
		UserID:  user.ID,
		Issuer:  idToken.Issuer,
		Subject: idToken.Subject,
	})
	if err != nil {
		return db.User{}, fmt.Errorf("failed to create OIDC identity: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return db.User{}, fmt.Errorf("failed to commit transaction: %w", err)
	}
	return user, nil
}

func verifyOIDC(ctx context.Context, googleOIDC config.GoogleOIDC, oidcCookies oidcCookies, code string) (*oidc.IDToken, error) {
	oauth2Token, err := googleOIDC.OAuth2Config.Exchange(
		ctx,
		code,
		oauth2.VerifierOption(oidcCookies.verifier.Value),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange Google OIDC authorization code: %w", err)
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return nil, errors.New("Google OIDC response did not contain an ID token")
	}

	idToken, err := googleOIDC.IDTokenVerifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("failed to verify Google OIDC ID token: %w", err)
	}

	if subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(oidcCookies.nonce.Value)) != 1 {
		return nil, errors.New("Google OIDC nonce mismatch")
	}
	return idToken, nil
}

func createLoginSession(ctx context.Context, queries db.Queries, user db.User) (cookieValues, error) {
	loginSessionTokenString, loginSessionTokenHash, err := generateLoginSessionToken()
	if err != nil {
		return cookieValues{}, fmt.Errorf(
			"failed to generate login session token: %w", err,
		)
	}

	expiresAt := time.Now().Add(LoginSessionDuration)

	if _, err = queries.CreateLoginSession(ctx, db.CreateLoginSessionParams{
		ID: pgtype.UUID{
			Bytes: uuid.NewV7(),
			Valid: true,
		},
		UserID:    user.ID,
		TokenHash: loginSessionTokenHash[:],
		ExpiresAt: pgtype.Timestamptz{
			Time:  expiresAt,
			Valid: true,
		},
	}); err != nil {
		return cookieValues{}, fmt.Errorf("failed to create login session: %w", err)
	}
	return cookieValues{value: loginSessionTokenString, expires: expiresAt, maxAge: int(LoginSessionDuration.Seconds())}, nil
}

func findOrCreateUser(ctx context.Context, dbPool *pgxpool.Pool, queries *db.Queries, logger *slog.Logger, idToken *oidc.IDToken, claims OIDCClaims) (db.User, error) {
	user, err := queries.GetUserByOIDCIdentity(ctx, db.GetUserByOIDCIdentityParams{
		Issuer:  idToken.Issuer,
		Subject: idToken.Subject,
	})
	if err == nil {
		return user, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, fmt.Errorf("failed to find user by OIDC identity: %w", err)
	}

	_, err = queries.GetUserByNormalizedEmail(ctx, strings.ToLower(strings.TrimSpace(claims.Email)))
	if err == nil {
		logger.Info("email is already associated with another identity")
		return db.User{}, errEmailAlreadyExists
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, fmt.Errorf("failed to find user by normalized email: %w", err)
	}

	user, err = createOIDCUser(dbPool, queries, ctx, idToken, claims)
	if err != nil {
		return db.User{}, fmt.Errorf("failed to create OIDC user: %w", err)
	}
	return user, nil
}
