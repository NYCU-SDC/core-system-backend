CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- User and Authentication Tables

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255),
    username VARCHAR(255),
    avatar_url VARCHAR(512) NOT NULL,
    role VARCHAR(255)[] NOT NULL DEFAULT '{"user"}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS user_emails (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    value VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id, value)
);

CREATE VIEW users_with_emails AS
SELECT
    u.id,
    u.name,
    u.username,
    u.avatar_url,
    u.role,
    u.created_at,
    u.updated_at,
    COALESCE(array_agg(e.value) FILTER (WHERE e.value IS NOT NULL), ARRAY[]::text[]) as emails
FROM users u
LEFT JOIN user_emails e ON u.id = e.user_id
GROUP BY u.id, u.name, u.username, u.avatar_url, u.role, u.created_at, u.updated_at;

CREATE TABLE auth (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider VARCHAR(255) NOT NULL,
    provider_id VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(provider, provider_id)
);

CREATE TABLE refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expiration_date TIMESTAMPTZ NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

-- Organizational And Unit Tables

CREATE TYPE unit_type AS ENUM ('organization', 'unit');

CREATE TABLE IF NOT EXISTS units (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID REFERENCES units(id),
    parent_id UUID REFERENCES units(id) ON DELETE SET NULL,
    type unit_type NOT NULL DEFAULT 'unit',
    name VARCHAR(255),
    description VARCHAR(255),
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_units_parent_id ON units(parent_id);

CREATE TABLE IF NOT EXISTS unit_members (
    unit_id UUID REFERENCES units(id) ON DELETE CASCADE,
    member_id UUID,
    PRIMARY KEY (unit_id, member_id)
);

CREATE TYPE db_strategy AS ENUM ('shared', 'isolated');

CREATE TABLE IF NOT EXISTS tenants (
    id          UUID PRIMARY KEY REFERENCES units (id) ON DELETE CASCADE,
    db_strategy db_strategy NOT NULL,
    owner_id    UUID REFERENCES users (id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS slug_history(
    id SERIAL PRIMARY KEY,
    slug TEXT NOT NULL,
    org_id UUID REFERENCES units(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at TIMESTAMPTZ DEFAULT null
);

-- Forms Tables

CREATE TYPE status AS ENUM(
    'draft',
    'published'
);

CREATE TYPE question_type AS ENUM(
    'short_text',
    'long_text',
    'single_choice',
    'multiple_choice',
    'date'
);

CREATE TABLE IF NOT EXISTS forms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title TEXT NOT NULL,
    description TEXT,
    preview_message TEXT,
    status status NOT NULL DEFAULT 'draft',
    deadline TIMESTAMPTZ,
    unit_id UUID REFERENCES units(id) ON DELETE CASCADE,
    last_editor UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS questions(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    form_id UUID NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
    required BOOLEAN NOT NULL,
    type question_type NOT NULL,
    title TEXT,
    description TEXT,
    metadata JSONB DEFAULT '{}'::JSONB,
    "order" INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TYPE response_progress AS ENUM (
    'draft',
    'submitted'
);

CREATE TABLE IF NOT EXISTS form_responses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    form_id UUID NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
    submitted_by UUID NOT NULL REFERENCES users(id),
    submitted_at TIMESTAMPTZ DEFAULT NULL,
    progress response_progress NOT NULL DEFAULT 'draft',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS answers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    response_id UUID NOT NULL REFERENCES form_responses(id) ON DELETE CASCADE,
    question_id UUID NOT NULL REFERENCES questions(id) ON DELETE CASCADE,
    type question_type NOT NULL,
    value TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TYPE content_type AS ENUM(
    'text',
    'form'
);

-- Inbox Tables

CREATE TABLE IF NOT EXISTS inbox_message(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    posted_by UUID NOT NULL references units(id),
    type content_type NOT NULL,
    content_id UUID NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS user_inbox_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL references users(id) ON DELETE CASCADE,
    message_id UUID NOT NULL references inbox_message(id) ON DELETE CASCADE,
    is_read boolean NOT NULL DEFAULT false,
    is_starred boolean NOT NULL DEFAULT false,
    is_archived boolean NOT NULL DEFAULT false
);
