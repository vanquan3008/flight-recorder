package postgres

// These SQL expressions share the same dependency identity in topology and
// detection. They only interpolate constants; request values remain parameters.
const dependencyTypeSQL = `CASE
 WHEN span_attributes ? 'db.system' OR span_attributes ? 'db.system.name' THEN 'database'
 WHEN span_attributes ? 'messaging.system' THEN 'queue' ELSE 'external' END`

const dependencyNameSQL = `CASE
 WHEN span_attributes ? 'db.system' OR span_attributes ? 'db.system.name' THEN
  COALESCE(NULLIF(span_attributes->>'db.namespace',''),NULLIF(span_attributes->>'db.name',''),
   NULLIF(span_attributes->>'server.address',''),NULLIF(span_attributes->>'db.system.name',''),span_attributes->>'db.system')
 WHEN span_attributes ? 'messaging.system' THEN
  COALESCE(NULLIF(span_attributes->>'messaging.destination.name',''),NULLIF(span_attributes->>'messaging.destination',''),span_attributes->>'messaging.system')
 ELSE COALESCE(NULLIF(span_attributes->>'peer.service',''),NULLIF(span_attributes->>'server.address',''),
  NULLIF(span_attributes->>'http.host',''),NULLIF(span_attributes->>'url.full','')) END`

const dependencyPredicateSQL = `(span_attributes ? 'db.system' OR span_attributes ? 'db.system.name'
 OR span_attributes ? 'messaging.system' OR span_kind=3)`
