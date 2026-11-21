-- name: CreateLoginSession :one
INSERT INTO login_sessions (id, user_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetLoginSessionByTokenHash :one
SELECT *
FROM login_sessions
WHERE token_hash = $1 AND expires_at > now();

-- name: TouchLoginSession :exec
UPDATE login_sessions SET last_seen_at = now() WHERE id = $1;

-- name: DeleteLoginSession :exec
DELETE FROM login_sessions WHERE id = $1;

-- name: DeleteLoginSessionsByUser :exec
DELETE FROM login_sessions WHERE user_id = $1;

-- name: DeleteExpiredLoginSessions :execrows
DELETE FROM login_sessions WHERE expires_at <= now();
