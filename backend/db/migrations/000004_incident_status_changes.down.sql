DROP TABLE IF EXISTS incident_status_changes;

ALTER TABLE api_keys
    DROP CONSTRAINT IF EXISTS api_keys_project_id_id_unique;
