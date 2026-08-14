# Canonical event model

Every ingestion adapter maps source data into `telemetry.Event`. OTLP protobuf messages stop at the adapter and never enter incident/domain APIs.

Required fields are event ID, project ID, service name, event type, and timestamp. Correlation identifiers, environment, version, duration, severity, status, operation, error data, bounded attributes, and an opaque payload reference are optional according to signal type.

Stable V1 types are `HTTP_SERVER`, `HTTP_CLIENT`, `LOG`, `EXCEPTION`, `DATABASE`, `CACHE`, `MESSAGE`, `RPC`, and `CUSTOM`. Unknown semantic types map to `CUSTOM` while retaining a bounded source-type attribute. Persistence uses strings so adding values does not reinterpret existing rows.

An Event becomes immutable after normalization, validation, and redaction. The bounded OTLP log body is retained as `log.body`; captured HTTP request/response bodies never live in attributes. Attributes have count, depth, key, value, and encoded-size limits. Large captured content uses an artifact reference after safe capture is implemented.

Correlation priority is TraceID, RequestID, then service plus a narrow time window. Timestamp and ID provide deterministic ordering for equal timestamps.
