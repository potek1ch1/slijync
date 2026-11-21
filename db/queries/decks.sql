-- name: CreateSlideDeck :one
INSERT INTO slide_decks (id, title, created_by_user_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetSlideDeckByID :one
SELECT * FROM slide_decks WHERE id = $1 AND deleted_at IS NULL;

-- name: GetSlideDeckIncludingDeleted :one
SELECT * FROM slide_decks WHERE id = $1;

-- name: ListSlideDecksForUser :many
SELECT sd.*, sdm.role
FROM slide_decks AS sd
JOIN slide_deck_members AS sdm ON sdm.slide_deck_id = sd.id
WHERE sdm.user_id = $1 AND sd.deleted_at IS NULL
ORDER BY sd.updated_at DESC, sd.id;

-- name: UpdateSlideDeckTitle :one
UPDATE slide_decks
SET title = $2, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteSlideDeck :one
UPDATE slide_decks
SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: CreatePDFVersion :one
INSERT INTO pdf_versions (
    id, slide_deck_id, version_number, storage_key, page_count,
    file_size_bytes, sha256, created_by_user_id
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetPDFVersionByID :one
SELECT * FROM pdf_versions WHERE id = $1;

-- name: ListPDFVersions :many
SELECT *
FROM pdf_versions
WHERE slide_deck_id = $1
ORDER BY version_number DESC;

-- name: SetCurrentPDFVersion :one
UPDATE slide_decks
SET current_pdf_version_id = $2, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: AddSlideDeckMember :one
INSERT INTO slide_deck_members (
    slide_deck_id, user_id, role, created_by_user_id
)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetSlideDeckMember :one
SELECT *
FROM slide_deck_members
WHERE slide_deck_id = $1 AND user_id = $2;

-- name: ListSlideDeckMembers :many
SELECT sdm.*, u.email, u.display_name
FROM slide_deck_members AS sdm
JOIN users AS u ON u.id = sdm.user_id
WHERE sdm.slide_deck_id = $1
ORDER BY sdm.created_at, sdm.user_id;

-- name: LockSlideDeckMembers :many
SELECT *
FROM slide_deck_members
WHERE slide_deck_id = $1
ORDER BY user_id
FOR UPDATE;

-- name: UpdateSlideDeckMemberRole :one
UPDATE slide_deck_members
SET role = $3
WHERE slide_deck_id = $1 AND user_id = $2
RETURNING *;

-- name: DeleteSlideDeckMember :execrows
DELETE FROM slide_deck_members
WHERE slide_deck_id = $1 AND user_id = $2;
