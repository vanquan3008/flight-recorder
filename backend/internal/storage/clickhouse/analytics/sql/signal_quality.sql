SELECT
    (
        SELECT count()
        FROM tracepilot_spans FINAL
        WHERE %s
    ) AS spans,
    (
        SELECT count()
        FROM (
            SELECT trace_id, parent_span_id
            FROM tracepilot_spans FINAL
            WHERE %s
              AND parent_span_id != ''
        ) AS child
        LEFT ANTI JOIN (
            SELECT trace_id, span_id
            FROM tracepilot_spans FINAL
            WHERE %s
        ) AS parent
            ON child.trace_id = parent.trace_id
           AND child.parent_span_id = parent.span_id
    ) AS orphan_spans,
    (
        SELECT count()
        FROM tracepilot_metric_points FINAL
        WHERE %s
    ) AS metric_points%s
FORMAT JSONEachRow
