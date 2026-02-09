CREATE TYPE unit_role AS ENUM ('admin', 'member');

ALTER TABLE unit_members
    ADD COLUMN role unit_role NOT NULL DEFAULT 'member';
