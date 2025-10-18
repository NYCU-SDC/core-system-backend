CREATE TYPE content_type AS ENUM(
    'text',
    'form'
);

CREATE TABLE IF NOT EXISTS inbox_message(
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    posted_by UUID NOT NULL references units(id) ON DELETE CASCADE,
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

