SELECT%s
FROM tracepilot_spans AS s FINAL
WHERE %s
GROUP BY s.trace_id%s
FORMAT JSONEachRow
