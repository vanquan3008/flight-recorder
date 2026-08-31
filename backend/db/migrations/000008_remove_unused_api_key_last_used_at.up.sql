DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM api_keys WHERE last_used_at IS NOT NULL) THEN
        RAISE EXCEPTION 'cannot remove api_keys.last_used_at with populated values';
    END IF;
END $$;

ALTER TABLE api_keys DROP COLUMN last_used_at;
