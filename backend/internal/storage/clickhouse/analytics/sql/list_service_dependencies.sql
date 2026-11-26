SELECT
    child.service_name AS service_name,
    count() AS call_count
FROM (
    SELECT trace_id, parent_span_id, service_name
    FROM tracepilot_spans FINAL
    WHERE project_id = {project:UUID}
      AND environment_id = {environment:UUID}
      AND started_at >= {from:DateTime64(9, 'UTC')}
      AND started_at <= {to:DateTime64(9, 'UTC')}
      AND parent_span_id != ''
      AND service_name != {service:String}
) AS child
INNER JOIN (
    SELECT trace_id, span_id
    FROM tracepilot_spans FINAL
    WHERE project_id = {project:UUID}
      AND environment_id = {environment:UUID}
      AND started_at >= {parent_from:DateTime64(9, 'UTC')}
      AND started_at <= {to:DateTime64(9, 'UTC')}
      AND service_name = {service:String}
) AS parent
    ON child.trace_id = parent.trace_id
   AND child.parent_span_id = parent.span_id
GROUP BY service_name
ORDER BY call_count DESC, service_name%s
FORMAT JSONEachRow
