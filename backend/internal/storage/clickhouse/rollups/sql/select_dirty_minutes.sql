SELECT
    toString(marks.project_id) AS project,
    toString(marks.environment_id) AS environment,
    marks.minute AS minute
FROM (
    SELECT project_id, environment_id, minute, max(marked_at) AS marked
    FROM tracepilot_span_minute_marks
    WHERE minute >= {lookback:DateTime64(9, 'UTC')}
      AND minute < toStartOfMinute({now:DateTime64(9, 'UTC')})
    GROUP BY project_id, environment_id, minute
) AS marks
LEFT JOIN (
    SELECT project_id, environment_id, minute, max(computed_at) AS computed
    FROM tracepilot_span_minute_done
    WHERE minute >= {lookback:DateTime64(9, 'UTC')}
    GROUP BY project_id, environment_id, minute
) AS done
    ON marks.project_id = done.project_id
   AND marks.environment_id = done.environment_id
   AND marks.minute = done.minute
WHERE marks.marked > done.computed
ORDER BY minute
LIMIT {limit:UInt32}
FORMAT JSONEachRow
