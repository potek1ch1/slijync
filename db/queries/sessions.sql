-- name: CreateSession :one
INSERT INTO sessions (id, slide_deck_id, created_by_user_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetSessionByID :one
SELECT * FROM sessions WHERE id = $1;

-- name: LockSessionByID :one
SELECT * FROM sessions WHERE id = $1 FOR UPDATE;

-- name: ListSessionsForSlideDeck :many
SELECT *
FROM sessions
WHERE slide_deck_id = $1
ORDER BY created_at DESC, id;

-- name: StartSession :one
UPDATE sessions
SET status = 'active',
    pdf_version_id = $2,
    started_at = now(),
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: EndSession :one
UPDATE sessions
SET status = 'ended', updated_at = now()
WHERE id = $1
RETURNING *;

-- name: CreateSessionRuntime :one
INSERT INTO session_runtimes (
    session_id, current_slide_number, revision, last_operated_at
)
VALUES ($1, 1, 1, now())
RETURNING *;

-- name: GetSessionRuntime :one
SELECT * FROM session_runtimes WHERE session_id = $1;

-- name: SetSessionSlide :one
UPDATE session_runtimes
SET current_slide_number = $2,
    revision = revision + 1,
    last_operated_at = now(),
    updated_at = now()
WHERE session_id = $1
RETURNING *;

-- name: DeleteSessionRuntime :execrows
DELETE FROM session_runtimes WHERE session_id = $1;

-- name: ListInactiveSessionRuntimes :many
SELECT *
FROM session_runtimes
WHERE last_operated_at <= $1
ORDER BY last_operated_at
FOR UPDATE SKIP LOCKED;
