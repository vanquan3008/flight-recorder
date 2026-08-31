CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX logs_body_trgm_idx ON logs USING GIN (
    (body::text) gin_trgm_ops
) WHERE body IS NOT NULL;
