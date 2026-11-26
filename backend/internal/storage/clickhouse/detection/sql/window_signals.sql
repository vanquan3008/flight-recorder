SELECT
    %s,%s
FROM tracepilot_spans FINAL
WHERE %s
  AND %s
GROUP BY %s%s
FORMAT JSONEachRow
