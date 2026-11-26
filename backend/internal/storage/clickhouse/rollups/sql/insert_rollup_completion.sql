INSERT INTO tracepilot_span_minute_done
SELECT
    {project:UUID},
    {environment:UUID},
    arrayJoin({minutes:Array(DateTime('UTC'))}),
    {computed:DateTime64(3, 'UTC')}
