CREATE TYPE db_strategy AS ENUM ('shared', 'isolated');

CREATE TABLE IF NOT EXISTS tenants
(
    id          UUID PRIMARY KEY REFERENCES units (id) ON DELETE CASCADE,
    slug        TEXT UNIQUE NOT NULL,
    db_strategy db_strategy NOT NULL,
    owner_id    UUID REFERENCES users (id) ON DELETE SET NULL
);