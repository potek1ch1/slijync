-- name: CreateUser :one
INSERT INTO users (id, email, normalized_email, display_name)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByNormalizedEmail :one
SELECT * FROM users WHERE normalized_email = $1;

-- name: UpdateUserProfile :one
UPDATE users
SET email = $2,
    normalized_email = $3,
    display_name = $4,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: CreateOIDCIdentity :one
INSERT INTO oidc_identities (id, user_id, issuer, subject)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUserByOIDCIdentity :one
SELECT u.*
FROM users AS u
JOIN oidc_identities AS oi ON oi.user_id = u.id
WHERE oi.issuer = $1 AND oi.subject = $2;
