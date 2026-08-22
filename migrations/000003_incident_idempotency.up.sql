CREATE UNIQUE INDEX incident_occurrences_project_event_idx
    ON incident_occurrences (project_id, event_id);
