-- +goose Up
CREATE TABLE users (
    id uuid PRIMARY KEY,
    email text NOT NULL,
    normalized_email text NOT NULL UNIQUE,
    display_name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_email_not_blank CHECK (btrim(email) <> ''),
    CONSTRAINT users_normalized_email_format CHECK (
        normalized_email = lower(btrim(normalized_email))
        AND normalized_email <> ''
    ),
    CONSTRAINT users_display_name_not_blank CHECK (btrim(display_name) <> '')
);

CREATE TABLE oidc_identities (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    issuer text NOT NULL,
    subject text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT oidc_identities_issuer_subject_key UNIQUE (issuer, subject),
    CONSTRAINT oidc_identities_issuer_not_blank CHECK (btrim(issuer) <> ''),
    CONSTRAINT oidc_identities_subject_not_blank CHECK (btrim(subject) <> '')
);

CREATE INDEX oidc_identities_user_id_idx ON oidc_identities(user_id);

CREATE TABLE login_sessions (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT login_sessions_token_hash_length CHECK (octet_length(token_hash) = 32)
);

CREATE INDEX login_sessions_user_id_idx ON login_sessions(user_id);
CREATE INDEX login_sessions_expires_at_idx ON login_sessions(expires_at);

CREATE TABLE slide_decks (
    id uuid PRIMARY KEY,
    title varchar(200) NOT NULL,
    current_pdf_version_id uuid,
    created_by_user_id uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    CONSTRAINT slide_decks_title_not_blank CHECK (btrim(title) <> '')
);

CREATE INDEX slide_decks_created_by_user_id_idx ON slide_decks(created_by_user_id);

CREATE TABLE pdf_versions (
    id uuid PRIMARY KEY,
    slide_deck_id uuid NOT NULL REFERENCES slide_decks(id),
    version_number integer NOT NULL,
    storage_key text NOT NULL UNIQUE,
    page_count integer NOT NULL,
    file_size_bytes bigint NOT NULL,
    sha256 bytea NOT NULL,
    created_by_user_id uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pdf_versions_deck_version_key UNIQUE (slide_deck_id, version_number),
    CONSTRAINT pdf_versions_version_number_positive CHECK (version_number >= 1),
    CONSTRAINT pdf_versions_storage_key_not_blank CHECK (btrim(storage_key) <> ''),
    CONSTRAINT pdf_versions_page_count_positive CHECK (page_count >= 1),
    CONSTRAINT pdf_versions_file_size_positive CHECK (file_size_bytes >= 1),
    CONSTRAINT pdf_versions_sha256_length CHECK (octet_length(sha256) = 32)
);

CREATE INDEX pdf_versions_slide_deck_id_idx ON pdf_versions(slide_deck_id);

ALTER TABLE slide_decks
    ADD CONSTRAINT slide_decks_current_pdf_version_id_fkey
    FOREIGN KEY (current_pdf_version_id) REFERENCES pdf_versions(id);

CREATE TABLE slide_deck_members (
    slide_deck_id uuid NOT NULL REFERENCES slide_decks(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role text NOT NULL,
    created_by_user_id uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (slide_deck_id, user_id),
    CONSTRAINT slide_deck_members_role_check CHECK (role IN ('owner', 'operator'))
);

CREATE INDEX slide_deck_members_user_id_idx ON slide_deck_members(user_id);

CREATE TABLE sessions (
    id uuid PRIMARY KEY,
    slide_deck_id uuid NOT NULL REFERENCES slide_decks(id),
    pdf_version_id uuid REFERENCES pdf_versions(id),
    status text NOT NULL DEFAULT 'waiting',
    created_by_user_id uuid NOT NULL REFERENCES users(id),
    started_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT sessions_status_check CHECK (status IN ('waiting', 'active', 'ended')),
    CONSTRAINT sessions_active_fields_check CHECK (
        status <> 'active' OR (pdf_version_id IS NOT NULL AND started_at IS NOT NULL)
    )
);

CREATE INDEX sessions_slide_deck_created_at_idx ON sessions(slide_deck_id, created_at DESC);

CREATE TABLE session_runtimes (
    session_id uuid PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
    current_slide_number integer NOT NULL,
    revision bigint NOT NULL,
    last_operated_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT session_runtimes_slide_number_positive CHECK (current_slide_number >= 1),
    CONSTRAINT session_runtimes_revision_positive CHECK (revision >= 1)
);

CREATE INDEX session_runtimes_last_operated_at_idx ON session_runtimes(last_operated_at);

-- +goose Down
DROP TABLE IF EXISTS session_runtimes;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS slide_deck_members;
ALTER TABLE IF EXISTS slide_decks DROP CONSTRAINT IF EXISTS slide_decks_current_pdf_version_id_fkey;
DROP TABLE IF EXISTS pdf_versions;
DROP TABLE IF EXISTS slide_decks;
DROP TABLE IF EXISTS login_sessions;
DROP TABLE IF EXISTS oidc_identities;
DROP TABLE IF EXISTS users;
