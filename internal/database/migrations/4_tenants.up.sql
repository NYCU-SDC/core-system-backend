CREATE TYPE db_strategy AS ENUM ('shared', 'isolated');

CREATE TABLE IF NOT EXISTS tenants
(
    id          UUID PRIMARY KEY REFERENCES units (id) ON DELETE CASCADE,
    slug        TEXT UNIQUE NOT NULL,
    db_strategy db_strategy NOT NULL,
    owner_id    UUID REFERENCES users (id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS history
(
    slug TEXT NOT NULL,
    org_id UUID REFERENCES units(id) ON DELETE CASCADE,
    orgName VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at TIMESTAMPTZ DEFAULT NULL,
    PRIMARY KEY (slug, org_id)
);