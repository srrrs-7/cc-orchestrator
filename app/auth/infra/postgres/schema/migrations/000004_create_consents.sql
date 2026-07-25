-- +goose Up
-- ISSUE-032: persist resource-owner consent grants (user × client × scope).
CREATE TABLE consents (
    user_id    text        NOT NULL,
    client_id  text        NOT NULL,
    scope      text        NOT NULL,
    granted_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, client_id, scope)
);

-- +goose Down
DROP TABLE consents;
