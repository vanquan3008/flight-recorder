-- Dependency identity of a span, shared by topology and detection queries so
-- both name the same database, queue or external target. Single-statement
-- IMMUTABLE SQL functions are inlined by the planner, so they cost nothing
-- per row compared with writing the expressions in each query.

-- A span calls a dependency when it is a database or messaging operation, or
-- any CLIENT span (kind 3).
CREATE FUNCTION span_is_dependency(span_attributes jsonb, span_kind integer)
RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$
    SELECT span_attributes ? 'db.system'
        OR span_attributes ? 'db.system.name'
        OR span_attributes ? 'messaging.system'
        OR span_kind = 3
$$;

CREATE FUNCTION span_dependency_type(span_attributes jsonb)
RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$
    SELECT CASE
        WHEN span_attributes ? 'db.system' OR span_attributes ? 'db.system.name' THEN 'database'
        WHEN span_attributes ? 'messaging.system' THEN 'queue'
        ELSE 'external'
    END
$$;

CREATE FUNCTION span_dependency_name(span_attributes jsonb)
RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE
AS $$
    SELECT CASE
        WHEN span_attributes ? 'db.system' OR span_attributes ? 'db.system.name' THEN
            COALESCE(
                NULLIF(span_attributes->>'db.namespace', ''),
                NULLIF(span_attributes->>'db.name', ''),
                NULLIF(span_attributes->>'server.address', ''),
                NULLIF(span_attributes->>'db.system.name', ''),
                span_attributes->>'db.system')
        WHEN span_attributes ? 'messaging.system' THEN
            COALESCE(
                NULLIF(span_attributes->>'messaging.destination.name', ''),
                NULLIF(span_attributes->>'messaging.destination', ''),
                span_attributes->>'messaging.system')
        ELSE
            COALESCE(
                NULLIF(span_attributes->>'peer.service', ''),
                NULLIF(span_attributes->>'server.address', ''),
                NULLIF(span_attributes->>'http.host', ''),
                NULLIF(span_attributes->>'url.full', ''))
    END
$$;
