SELECT%s,
    any(p.last_match) AS cursor_at
FROM tracepilot_spans AS s FINAL
INNER JOIN (%s
) AS p ON s.trace_id = p.trace_id
WHERE %s
GROUP BY s.trace_id
HAVING %s
ORDER BY cursor_at DESC, id DESC
LIMIT {limit:UInt32}%s
FORMAT JSONEachRow
