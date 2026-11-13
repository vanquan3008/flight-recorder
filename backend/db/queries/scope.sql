-- The configured project and environment every ingested record belongs to.
-- name: EnsureIngestionScope :exec
WITH project AS (
    INSERT INTO projects (id, slug, name)
    VALUES (@project_id, @project_slug, @project_name)
    ON CONFLICT (id) DO UPDATE
    SET slug = EXCLUDED.slug, name = EXCLUDED.name, updated_at = now()
    RETURNING id
)
INSERT INTO environments (id, project_id, name)
SELECT @environment_id, project.id, @environment_name FROM project
ON CONFLICT (project_id, id) DO UPDATE SET name = EXCLUDED.name;
