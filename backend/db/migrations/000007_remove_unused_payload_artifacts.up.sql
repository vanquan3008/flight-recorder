DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM events WHERE payload_ref IS NOT NULL)
        OR EXISTS (SELECT 1 FROM replay_executions WHERE result_payload_ref IS NOT NULL) THEN
        RAISE EXCEPTION 'cannot remove payload artifacts with populated references';
    END IF;
END
$$;

ALTER TABLE events
    DROP CONSTRAINT IF EXISTS events_project_id_payload_ref_fkey,
    DROP COLUMN payload_ref;

ALTER TABLE replay_executions
    DROP CONSTRAINT IF EXISTS replay_executions_project_id_result_payload_ref_fkey,
    DROP COLUMN result_payload_ref;

DROP TABLE payload_artifacts;
