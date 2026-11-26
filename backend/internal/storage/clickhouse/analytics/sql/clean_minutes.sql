SELECT marks.minute AS minute
FROM (
    SELECT minute, max(marked_at) AS marked
    FROM tracepilot_span_minute_marks
    WHERE project_id = {project:UUID}
      AND environment_id = {environment:UUID}
      AND minute >= {clean_from:DateTime64(9, 'UTC')}
      AND minute + INTERVAL 1 MINUTE <= {clean_to:DateTime64(9, 'UTC')}
    GROUP BY minute
) AS marks
INNER JOIN (
    SELECT minute, max(computed_at) AS computed
    FROM tracepilot_span_minute_done
    WHERE project_id = {project:UUID}
      AND environment_id = {environment:UUID}
      AND minute >= {clean_from:DateTime64(9, 'UTC')}
      AND minute + INTERVAL 1 MINUTE <= {clean_to:DateTime64(9, 'UTC')}
    GROUP BY minute
) AS done ON marks.minute = done.minute
WHERE done.computed >= marks.marked
