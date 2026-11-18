ALTER TABLE projects ADD COLUMN telemetry_retention_days integer
    CHECK (telemetry_retention_days BETWEEN 1 AND 106751);
