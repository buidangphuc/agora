CREATE TABLE greetings (
    id         bigserial PRIMARY KEY,
    name       text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
