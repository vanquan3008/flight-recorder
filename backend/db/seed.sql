-- Dữ liệu mẫu để xem thử giao diện. Chỉ chạy trên database dùng để thử, không chạy production.
--
--   psql "$TRACEPILOT_DATABASE_URL" -f backend/db/seed.sql
--
-- Chạy sau `tracepilot migrate up`. Chạy lại được nhiều lần: mỗi lần xóa dữ liệu mẫu cũ
-- rồi tạo lại, với thời gian tính từ lúc chạy nên luôn nằm trong khung 15 phút gần nhất.
-- Dữ liệu thuộc project/environment mặc định (default / development).

BEGIN;

CREATE TEMP TABLE seed_time ON COMMIT DROP AS
SELECT date_trunc('minute', now()) - interval '4 minutes' AS base;

-- Project và environment mặc định (giống lúc server khởi động).
INSERT INTO projects (id, slug, name)
VALUES ('01900000-0000-7000-8000-000000000001', 'default', 'Default Project')
ON CONFLICT (id) DO NOTHING;
INSERT INTO environments (id, project_id, name)
VALUES ('01900000-0000-7000-8000-000000000002', '01900000-0000-7000-8000-000000000001', 'development')
ON CONFLICT (project_id, id) DO NOTHING;

-- Tài khoản quản trị để đăng nhập ngay: admin@tracepilot.local / admin-password-123.
-- Là super admin và owner của project mặc định. Chỉ để thử trên máy dev: đổi mật khẩu hoặc xóa
-- tài khoản này trước khi dùng thật. Chạy lại không đặt lại mật khẩu đã đổi.
INSERT INTO users (id, email, display_name, password_hash, platform_admin)
VALUES ('5eed0000-0000-4000-8000-0000000000ad', 'admin@tracepilot.local', 'Admin',
        'pbkdf2-sha256$600000$FebVOQydajic2r6kpocCfw$6+9j0Sm9rMjtFp90oTSPgqmWRwRuYmiIF9u6bI/cCzs', true)
ON CONFLICT (email) DO NOTHING;
INSERT INTO project_members (project_id, user_id, role)
SELECT '01900000-0000-7000-8000-000000000001', id, 'owner' FROM users WHERE email = 'admin@tracepilot.local'
ON CONFLICT (project_id, user_id) DO NOTHING;

-- Xóa dữ liệu mẫu của lần chạy trước (nhận biết bằng id cố định).
DELETE FROM incidents WHERE id IN ('5eed0000-0000-4000-8000-0000000000a1', '5eed0000-0000-4000-8000-0000000000a2');
DELETE FROM events WHERE id::text LIKE '5eed0000-0000-4000-8000-0000000001%';
DELETE FROM spans  WHERE event_id::text LIKE '5eed0000-0000-4000-8000-0000000001%';
DELETE FROM logs   WHERE event_id::text LIKE '5eed0000-0000-4000-8000-0000000001%';

INSERT INTO services (id, project_id, environment_id, name)
VALUES ('5eed0000-0000-4000-8000-000000000011', '01900000-0000-7000-8000-000000000001', '01900000-0000-7000-8000-000000000002', 'checkout'),
       ('5eed0000-0000-4000-8000-000000000012', '01900000-0000-7000-8000-000000000001', '01900000-0000-7000-8000-000000000002', 'payment'),
       ('5eed0000-0000-4000-8000-000000000013', '01900000-0000-7000-8000-000000000001', '01900000-0000-7000-8000-000000000002', 'catalog')
ON CONFLICT DO NOTHING;

-- Các span. event_id nhóm 01..04. Ba trace:
--   checkout: GET /checkout (OK) gọi payment POST /charge (lỗi timeout)
--   catalog:  GET /products (OK)
--   incident: POST /checkout (lỗi 500), kèm một log ERROR
CREATE TEMP TABLE seed_spans ON COMMIT DROP AS
SELECT * FROM (VALUES
  ('5eed0000-0000-4000-8000-000000000101'::uuid, 'checkout', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01', 'bbbbbbbbbbbbbbbb', NULL::text,        'GET /checkout',  2, 0,   120, 'OK',    200, 'HTTP_SERVER', NULL::text,        NULL::text),
  ('5eed0000-0000-4000-8000-000000000102'::uuid, 'payment',  'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01', 'aaaaaaaaaaaaaaaa', 'bbbbbbbbbbbbbbbb', 'POST /charge',   3, 80,  900, 'ERROR', 502, 'HTTP_CLIENT', 'UpstreamTimeout', 'payment provider timed out'),
  ('5eed0000-0000-4000-8000-000000000103'::uuid, 'catalog',  'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa02', 'cccccccccccccccc', NULL,              'GET /products',  2, 60000, 45, 'OK',  200, 'HTTP_SERVER', NULL,              NULL),
  ('5eed0000-0000-4000-8000-000000000104'::uuid, 'checkout', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa03', 'dddddddddddddddd', NULL,              'POST /checkout', 2, 120000, 250, 'ERROR', 500, 'HTTP_SERVER', 'CheckoutError', 'checkout dependency failed')
) AS t(event_id, service, trace_id, span_id, parent_span_id, name, kind, start_ms, duration_ms, status, http_status, event_type, error_type, error_message);

INSERT INTO events (id, project_id, environment_id, service_name, trace_id, span_id, parent_span_id, event_type,
                    occurred_at, duration_ns, status, http_status, operation, error_type, error_message, attributes)
SELECT s.event_id, '01900000-0000-7000-8000-000000000001', '01900000-0000-7000-8000-000000000002', s.service,
       s.trace_id, s.span_id, s.parent_span_id, s.event_type,
       t.base + s.start_ms * interval '1 millisecond', s.duration_ms * 1000000, s.status, s.http_status, s.name,
       s.error_type, s.error_message, '{"source":"tracepilot-sample"}'::jsonb
FROM seed_spans s, seed_time t;

INSERT INTO spans (event_id, project_id, environment_id, trace_id, span_id, parent_span_id, span_name, span_kind,
                   started_at, ended_at, status_code, status_message, service_name, resource_attributes, span_attributes)
SELECT s.event_id, '01900000-0000-7000-8000-000000000001', '01900000-0000-7000-8000-000000000002', s.trace_id, s.span_id,
       s.parent_span_id, s.name, s.kind,
       t.base + s.start_ms * interval '1 millisecond',
       t.base + (s.start_ms + s.duration_ms) * interval '1 millisecond',
       s.status, COALESCE(s.error_message, ''), s.service,
       jsonb_build_object('service.name', s.service, 'deployment.environment', 'development'),
       CASE WHEN s.kind = 3 THEN '{"http.url":"https://payments.example.test/charge","peer.service":"payment-provider","source":"tracepilot-sample"}'::jsonb
            ELSE '{"source":"tracepilot-sample"}'::jsonb END
FROM seed_spans s, seed_time t;

-- Hai log, event_id nhóm 05..06.
INSERT INTO events (id, project_id, environment_id, service_name, trace_id, event_type, occurred_at, severity, attributes)
SELECT l.id, '01900000-0000-7000-8000-000000000001', '01900000-0000-7000-8000-000000000002', l.service, l.trace_id, 'LOG',
       t.base + l.at_ms * interval '1 millisecond', l.severity, '{"source":"tracepilot-sample"}'::jsonb
FROM (VALUES
  ('5eed0000-0000-4000-8000-000000000105'::uuid, 'catalog',  'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa02', 'INFO',  60050),
  ('5eed0000-0000-4000-8000-000000000106'::uuid, 'checkout', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa03', 'ERROR', 120250)
) AS l(id, service, trace_id, severity, at_ms), seed_time t;

INSERT INTO logs (event_id, project_id, environment_id, occurred_at, trace_id, trace_flags, severity_number, severity_text,
                  body, service_name, resource_attributes, log_attributes)
SELECT l.id, '01900000-0000-7000-8000-000000000001', '01900000-0000-7000-8000-000000000002',
       t.base + l.at_ms * interval '1 millisecond', l.trace_id, 0, l.severity_number, l.severity,
       jsonb_build_object('message', l.message), l.service,
       jsonb_build_object('service.name', l.service, 'deployment.environment', 'development'),
       '{"source":"tracepilot-sample"}'::jsonb
FROM (VALUES
  ('5eed0000-0000-4000-8000-000000000105'::uuid, 'catalog',  'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa02', 'INFO',   9, 'catalog cache refreshed',  60050),
  ('5eed0000-0000-4000-8000-000000000106'::uuid, 'checkout', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa03', 'ERROR', 17, 'checkout returned 500',   120250)
) AS l(id, service, trace_id, severity, severity_number, message, at_ms), seed_time t;

-- Hai incident cho hai span lỗi, kèm bằng chứng để xem được phân tích nguyên nhân.
INSERT INTO incidents (id, project_id, environment_id, primary_service, trace_id, fingerprint, title, error_type, error_message,
                       http_status, severity, status, first_seen_at, last_seen_at, occurrence_count)
SELECT i.id, '01900000-0000-7000-8000-000000000001', '01900000-0000-7000-8000-000000000002', i.service, i.trace_id,
       i.fingerprint, i.title, i.error_type, i.error_message, i.http_status, 'ERROR', 'OPEN',
       t.base + i.at_ms * interval '1 millisecond', t.base + i.at_ms * interval '1 millisecond', 1
FROM (VALUES
  ('5eed0000-0000-4000-8000-0000000000a1'::uuid, 'payment',  'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa01', 'sample-payment-upstream-timeout', 'payment POST /charge failed',  'UpstreamTimeout', 'payment provider timed out',   502, 80),
  ('5eed0000-0000-4000-8000-0000000000a2'::uuid, 'checkout', 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa03', 'sample-checkout-error',            'checkout POST /checkout failed', 'CheckoutError',   'checkout dependency failed', 500, 120000)
) AS i(id, service, trace_id, fingerprint, title, error_type, error_message, http_status, at_ms), seed_time t;

INSERT INTO incident_occurrences (id, project_id, incident_id, event_id, occurred_at)
SELECT m.occurrence_id, '01900000-0000-7000-8000-000000000001', m.incident_id, m.event_id, e.occurred_at
FROM (VALUES
  ('5eed0000-0000-4000-8000-0000000002a1'::uuid, '5eed0000-0000-4000-8000-0000000000a1'::uuid, '5eed0000-0000-4000-8000-000000000102'::uuid),
  ('5eed0000-0000-4000-8000-0000000002a2'::uuid, '5eed0000-0000-4000-8000-0000000000a2'::uuid, '5eed0000-0000-4000-8000-000000000104'::uuid)
) AS m(occurrence_id, incident_id, event_id)
JOIN events e ON e.id = m.event_id;

INSERT INTO incident_events (project_id, incident_id, event_id)
VALUES ('01900000-0000-7000-8000-000000000001', '5eed0000-0000-4000-8000-0000000000a1', '5eed0000-0000-4000-8000-000000000102'),
       ('01900000-0000-7000-8000-000000000001', '5eed0000-0000-4000-8000-0000000000a2', '5eed0000-0000-4000-8000-000000000104');

INSERT INTO incident_evidence (id, project_id, environment_id, incident_id, signal_type, entity_type, entity_id, metric_name,
                               direction, observed_value, baseline_value, change_ratio, window_started_at, window_ended_at,
                               trace_ids, summary)
SELECT m.evidence_id, '01900000-0000-7000-8000-000000000001', '01900000-0000-7000-8000-000000000002', m.incident_id,
       'event', 'service', m.entity_id, 'error_event', 'event', 1, 0, 1, e.occurred_at, e.occurred_at,
       ARRAY[e.trace_id], e.error_message
FROM (VALUES
  ('5eed0000-0000-4000-8000-0000000003a1'::uuid, '5eed0000-0000-4000-8000-0000000000a1'::uuid, '5eed0000-0000-4000-8000-000000000102'::uuid, 'service:payment'),
  ('5eed0000-0000-4000-8000-0000000003a2'::uuid, '5eed0000-0000-4000-8000-0000000000a2'::uuid, '5eed0000-0000-4000-8000-000000000104'::uuid, 'service:checkout')
) AS m(evidence_id, incident_id, event_id, entity_id)
JOIN events e ON e.id = m.event_id;

COMMIT;
