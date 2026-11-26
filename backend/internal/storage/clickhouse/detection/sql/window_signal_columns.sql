
    countIf(started_at >= {current:DateTime64(9, 'UTC')}) AS current_calls,
    countIf(started_at < {current:DateTime64(9, 'UTC')}) AS baseline_calls,
    countIf(started_at >= {current:DateTime64(9, 'UTC')} AND status_code = 'ERROR') AS current_errors,
    countIf(started_at < {current:DateTime64(9, 'UTC')} AND status_code = 'ERROR') AS baseline_errors,
    if(current_calls = 0, 0,
       sumIf(duration_ns, started_at >= {current:DateTime64(9, 'UTC')}) / current_calls) AS current_latency_nanos,
    if(baseline_calls = 0, 0,
       sumIf(duration_ns, started_at < {current:DateTime64(9, 'UTC')}) / baseline_calls) AS baseline_latency_nanos,
    arraySlice(arraySort(groupUniqArrayIf(trace_id, started_at >= {current:DateTime64(9, 'UTC')})), 1, 5) AS trace_ids
