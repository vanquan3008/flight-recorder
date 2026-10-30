# Điều kiện vận hành production

Mục tiêu hiện tại: một TracePilot replica, PostgreSQL riêng trên mạng tin cậy,
TLS cho management/OTLP và database, image cố định bằng digest.
Đây là runbook nghiệm thu, không phải tuyên bố source đã được chứng nhận production.
Việc còn lại và giới hạn đã biết ở mục cuối tài liệu này.

## Checklist trước khi mở traffic

Chỉ mở traffic khi mọi dòng có evidence trên commit/image digest sẽ phát hành.
Các giá trị do chủ repo chốt, không suy ra từ default:

| Hạng mục | Cần có | Ở đâu |
| --- | --- | --- |
| Gate tự động | CI xanh trên commit release (web, lint, test + E2E, docker) | [CI](../.github/workflows/ci.yml) |
| Image | Digest cố định, chạy nonroot, healthcheck đạt | Preflight bước 2, 5 |
| Dữ liệu | DB mới hoặc đường data migration đã kiểm chứng cho DB cũ | Preflight bước 1 |
| Phục hồi | RPO/RTO đã chốt; restore thử vào DB riêng đạt | Chứng minh dữ liệu |
| Retention | Bốn max_age đã xem lại; xem trước lượng sẽ bị xóa; dung lượng đĩa | Retention |
| Tải | Peak, p95/p99, RSS đo trên cardinality đại diện | Tải và giới hạn bộ nhớ |
| Metrics | OTLP metrics ACK sau commit như trace/log (không còn best-effort) | Tải và giới hạn bộ nhớ |
| Bảo mật | TLS verify-full, key tối thiểu quyền, bootstrap đã revoke | Preflight bước 4–6 |
| Rollback | Image cũ tương thích schema hiện tại; migrate down không dùng | Chứng minh dữ liệu |

## Preflight và triển khai

1. Xác định có DB cũ không. Source hiện chỉ có baseline schema 1, không nâng cấp
   tại chỗ DB dùng lịch sử migration cũ. Giữ backup và binary tương thích cho DB đó.
2. Dùng một file `.env` ở root, lấy `.env.example` làm mẫu. Đặt
   `TRACEPILOT_PRODUCTION=true`, `TRACEPILOT_DATABASE_URL` trỏ DB ngoài và chạy image
   cố định bằng release digest. Dùng bootstrap key riêng đủ mạnh; tạo key ingest/read
   riêng theo nhiệm vụ.
3. Xem lại bốn giá trị retention (mục Retention) và dung lượng đĩa theo volume thực.
   Mặc định đã bật xóa tự động; 0s trên một nhóm giữ nhóm đó mãi.
4. PostgreSQL dùng sslmode=verify-full và hostname khớp certificate. Với CA riêng,
   mount CA read-only trong container và đặt sslrootcert tới đường dẫn đó.
   Validator hiện vẫn chấp nhận require/verify-ca; đó không thay thế việc chọn
   verify-full cho deployment này.
5. Thư mục TLS chứa tls.crt, tls.key, ca.crt, cho UID nonroot đọc được; mount read-only
   vào container tại /run/tracepilot/tls và bật các biến TLS/healthcheck cuối `.env.example`.
   HEALTHCHECK_SERVER_NAME phải khớp DNS certificate. Chỉ mở cổng tới interface tin cậy.
6. Chạy migrate riêng trên DB đã xác minh, rồi chạy server (cùng một image, cùng `.env`):

```powershell
docker pull ghcr.io/vanquan3008/tracepilot@sha256:<release-digest>
docker run --rm --env-file .env <image> migrate up
docker run -d --name tracepilot --env-file .env -v <thư-mục-tls>:/run/tracepilot/tls:ro `
  -p 8080:8080 -p 4317:4317 -p 4318:4318 --restart unless-stopped --stop-timeout 35 <image>
```

Host `localhost` trong `TRACEPILOT_DATABASE_URL` nghĩa là chính container; dùng tên host hay IP
mà container tới được. Image chạy nonroot; thêm `--read-only --tmpfs /tmp
--security-opt no-new-privileges` để khóa chặt hơn.

Kiểm tra healthcheck container và /health/ready qua HTTPS tin cậy. Query một
trace/log bằng key read; gửi thử bằng key ingest. /metrics yêu cầu quyền read.
Không ghi config có secret, API key hoặc raw telemetry vào log vận hành.
Revoke bootstrap sau khi đã có admin key thay thế; restart cùng bootstrap key
không tự bỏ trạng thái revoke.

Mọi cấu hình đi qua một file `.env` (hoặc biến môi trường của container, có ưu tiên cao
hơn). Nếu dùng secret manager, cấp đúng file này hoặc đặt biến môi trường tương ứng.

## Chứng minh dữ liệu và phục hồi

Trước release, chạy trên staging có thể bỏ đi và lưu evidence theo image digest:

| Drill | Điều kiện đạt |
| --- | --- |
| Schema | Up → down 1 → up trên DB sạch (down 1 xóa cả schema baseline) và integration suite pass; startup từ chối schema dirty/sai version. |
| Restore | Backup phục hồi vào DB khác; so row counts, scope, sample trace/log/incident/evidence/metrics/outbox; start server trên DB restore rồi kiểm tra readiness/API. |
| Auth/storage outage | Ngắt DB test, OTLP trả retryable, không ACK trước commit; reconnect, retry cùng payload không tăng unique rows. |
| Shutdown | Gửi tải và SIGTERM; ACK chỉ cho dữ liệu commit, không vượt grace, không panic/deadlock; exporter phục hồi phần chưa ACK. |
| Webhook outage | Endpoint test trả 503; retry/outbox tồn tại qua restart, stale sending lease được reclaim, cuối cùng succeed/failed đúng policy. |
| TLS | Certificate chain, DNS/SAN và expiry hợp lệ cho client, healthcheck và PostgreSQL. |

Dùng pg_dump/pg_restore hoặc backup/PITR của managed database; định kỳ thực sự
restore vào DB riêng. Chốt RPO (mức dữ liệu được phép mất) và RTO (thời gian
khôi phục) trước khi triển khai. Chỉ rollback image nếu schema tương thích; migrate down không phải cách rollback
production vì baseline chỉ có một bước và down xóa toàn bộ schema. Muốn đổi schema,
thêm migration mới tương thích với image trước (chỉ thêm, không xóa hay đổi nghĩa
cột) để rollback image không cần động đến database.

Mỗi kết nối pool có `TRACEPILOT_DATABASE_STATEMENT_TIMEOUT` (30s, phải không thấp
hơn `HTTP_REQUEST_TIMEOUT` để API trả 504 trước), `..._LOCK_TIMEOUT` (10s) và
`..._IDLE_IN_TRANSACTION_TIMEOUT` (30s); 0 giữ mặc định của server, tham số cùng tên
trên URL được ưu tiên. Kết nối hiện `application_name=tracepilot` trong
`pg_stat_activity`. Nếu bảo trì partition hoặc retention báo hết hạn khóa/lệnh trên
bảng lớn, tăng giá trị thay vì tắt.

Repo hiện không có helper tự động hóa migration rollback drill. Chỉ thử nâng cấp
và rollback trên database disposable sau khi xác minh URL; cần kiểm tra tương
thích application N/N-1 bằng image thật trong staging.

## Tải và giới hạn bộ nhớ

Chốt events/giây, metric points/giây, kích thước payload, concurrent exporters,
cardinality, retention và p95/p99 ACK trước khi đặt SLO. Chạy 2x peak dự kiến,
ghi CPU/RSS, queue, DB pool/WAL, disk/index growth và latency truy vấn.

Repo hiện không có load-test runner. Đo lại trên staging/DB disposable bằng một
harness được kiểm soát; không coi số liệu lịch sử hoặc ví dụ threshold là SLO.
Đối chiếu số rows và counter storage với expected count; partial rejection là
failure. Retry/idempotency cần drill riêng bằng cách gửi lại chính payload cũ.

Queue byte budget là ước lượng và áp dụng riêng cho events/metrics. Nó không
giới hạn tổng RSS; đặc biệt gRPC decode xảy ra trước admission handler. OTLP
metrics ACK sau commit giống trace/log: lỗi DB trả 503/Unavailable để exporter
gửi lại, điểm lưu được không bị mất sau ACK. Chỉ metrics Prometheus của chính
TracePilot (`/metrics`) nằm trong bộ nhớ và mất khi tiến trình chết.

Rate limit mặc định (120 request/phút mỗi IP) thấp hơn tải đo; nâng
`TRACEPILOT_HTTP_RATE_LIMIT_REQUESTS` chỉ cho phiên đo trên staging. Kết quả đã
đo và giới hạn môi trường ở mục "Việc còn lại" cuối tài liệu.

Cổng OTLP đếm xác thực thất bại theo địa chỉ kết nối: quá
`TRACEPILOT_INGESTION_AUTH_FAILURE_LIMIT` (60) lần trong cửa sổ
`TRACEPILOT_INGESTION_AUTH_FAILURE_WINDOW` (1m) thì trả 429 (gRPC RESOURCE_EXHAUSTED)
không truy vấn DB, và key bị từ chối được nhớ 5 giây. Key hợp lệ không bao giờ được
cache nên revoke có hiệu lực ngay. Sau reverse proxy hoặc NAT mọi client dùng chung
một địa chỉ (OTLP chưa đọc X-Forwarded-For), nên giữ giới hạn rộng; theo dõi
`tracepilot_otlp_requests_total{outcome="throttled"}`. Event đơn lẻ vượt ngân sách byte
của queue được báo là rejected (partial success) thay vì 503, vì gửi lại không giúp ích.

API quản trị chạy tối đa `TRACEPILOT_HTTP_MAX_CONCURRENT_QUERIES` (4) request cùng
lúc, mỗi request tối đa `TRACEPILOT_HTTP_REQUEST_TIMEOUT` (20s); phần vượt trả
503 + Retry-After, quá hạn trả 504. Hai giá trị này chia pool DB giữa đọc với
ingest và worker; chỉnh theo số core của PostgreSQL.

## Retention, thông báo và cảnh báo

Retention chạy theo nhóm, mỗi nhóm một max_age; `0s` giữ nhóm đó mãi:

| Nhóm | Biến | Mặc định | Xóa gì |
| --- | --- | --- | --- |
| Telemetry thô | `TRACEPILOT_RETENTION_MAX_AGE` | 168h (7 ngày) | events, spans, logs, metric points (drop partition theo ngày) |
| Incident | `TRACEPILOT_RETENTION_INCIDENT_MAX_AGE` | 2160h (90 ngày) | incident RESOLVED/IGNORED không hoạt động, kèm evidence, occurrence, lịch sử trạng thái, replay result |
| Audit | `TRACEPILOT_RETENTION_AUDIT_MAX_AGE` | 4320h (180 ngày) | API key audit events |
| Webhook delivery | `TRACEPILOT_RETENTION_DELIVERY_MAX_AGE` | 720h (30 ngày) | delivery đã succeeded/failed |

Giữ lại luôn: incident OPEN/INVESTIGATING (dù cũ), incident vừa hoạt động trong
cửa sổ, delivery pending/sending. Telemetry dùng ngày UTC nên có thể giữ thêm gần
một ngày. Evidence còn tham chiếu trace đã hết hạn; UI báo "trace không còn".
Lịch sử đổi trạng thái là một phần của incident nên sống cùng incident (≥90 ngày),
ngắn hơn audit key 180 ngày; muốn 180 ngày cho nó cần migration bỏ cascade.

**Trước khi nâng cấp DB đã có dữ liệu**, bản này bật xóa ở chu kỳ đầu tiên. Xem
trước lượng sẽ bị xóa (chỉ đọc):

```sql
SELECT (SELECT count(*) FROM events WHERE occurred_at < now() - interval '7 days') AS events,
       (SELECT count(*) FROM incidents WHERE status IN ('RESOLVED','IGNORED') AND last_seen_at < now() - interval '90 days' AND updated_at < now() - interval '90 days') AS incidents,
       (SELECT count(*) FROM api_key_events WHERE created_at < now() - interval '180 days') AS audit,
       (SELECT count(*) FROM webhook_deliveries WHERE status IN ('succeeded','failed') AND updated_at < now() - interval '30 days') AS deliveries;
```

Muốn giữ như cũ, đặt các biến trên về `0s` trước khi khởi động.

## Nhiều replica

Thiết kế cho nhiều replica dùng chung PostgreSQL, không cần leader election; mỗi
bất biến dựa vào ràng buộc hoặc thao tác nguyên tử của PostgreSQL và có test
(`replicas_integration_test.go`, `idempotent_inserts_integration_test.go`):
ingest trùng được chống bằng ID tất định + `ON CONFLICT DO NOTHING`; outbox claim
bằng `FOR UPDATE SKIP LOCKED` và lease 6 phút nên crash được nhận lại; detection
cùng cửa sổ phút tạo một incident; tạo partition và provisioning lúc khởi động
tuần tự hóa bằng advisory lock theo transaction. Cần lưu ý:

- Tổng connection = số replica x `database.max_connections` (mặc định 20) phải nhỏ
  hơn `max_connections` của PostgreSQL.
- Rate limit và giới hạn truy vấn đồng thời tính theo từng tiến trình, nên giới
  hạn toàn hệ thống là N lần. Cần giới hạn chung thì đặt ở reverse proxy.
- Phía sau reverse proxy, khai báo `TRACEPILOT_HTTP_TRUSTED_PROXIES` (CIDR hoặc IP
  của proxy). Để trống thì `X-Forwarded-For` bị bỏ qua; khi khai báo, địa chỉ client
  lấy từ phải sang, bỏ qua các proxy tin cậy. Không có cache xác thực: key bị
  revoke/rotate có hiệu lực ngay trên mọi replica.

Webhook chỉ bật khi cần, kèm encryption key ngẫu nhiên 32 byte hex/base64 từ
secret manager. URL phải HTTPS; worker kiểm tra DNS/IP, chặn private/metadata
ranges và không theo redirect. Receiver cần xử lý delivery ID trùng do cơ chế
at-least-once. Replay giữ tắt trừ khi đã chốt allowlist và quyền vận hành.

Theo dõi readiness, queue depth/inflight bytes/events, rejected theo reason,
storage/auth errors, detection duration/failures, retention backlog, webhook
failure/claim errors, backup age, disk, TLS expiry và RSS. Khi shutdown, xem
tracepilot_ingestion_shutdowns_total theo completed/timeout.
Stop grace mặc định 35s cho hai budget 15s; tăng grace nếu đổi shutdown_timeout.

Chưa có phép đo tải hoặc SLO được duyệt. Chỉ mở production traffic sau khi
các drill, CI trên commit release và policy dữ liệu ở trên có evidence.

## Việc còn lại trước production và giới hạn đã biết

Kiểm tra gần nhất (30/10/2026, máy phát triển, PostgreSQL 17 dùng một lần): unit,
architecture và integration test pass, staticcheck 2026.1 sạch, migration up/down/up
version 1 đạt, collection Postman 41/41 trên server thật, frontend typecheck và 64 test
pass. **Chưa chạy:** `-race` (cần CGO), Playwright e2e lần cuối, govulncheck, soak/tải,
backup/restore, TLS, nhiều replica trên Linux staging. Không gọi các kết quả trên là
chứng nhận production.

| # | Việc | Trạng thái và điều kiện xong |
| --- | --- | --- |
| 1 | Peak tải, SLO p95/p99, RSS, tăng trưởng đĩa | Chưa đo. Công cụ tải cũ đã bị xóa khỏi repo; dùng `telemetrygen` theo mục "Đo tải" bên dưới, đối chiếu số dòng lưu với số đã ACK. |
| 2 | RPO/RTO và restore | Chưa chốt, chưa diễn tập. Restore vào DB riêng, so row count và gọi thử API. |
| 3 | DB cũ | Baseline hiện không nâng cấp tại chỗ DB dùng lịch sử migration cũ. Hỏi rõ dữ liệu đang có; không tự recreate. |
| 4 | Endpoint tổng hợp (services, topology, danh sách trace, performance) | **Đã giảm phần dễ:** tra parent/span cùng trace giới hạn theo thời gian (±1 giờ) thay vì dò mọi partition, và index theo thời gian cho `spans` (`spans_window_idx`). Trên 2M span: topology 176 → 31 ms, danh sách trace 47 → 15 ms, kiểm tra chất lượng 100 → 31 ms, cửa sổ phát hiện 28 → 12 ms. Vẫn quét toàn bộ span của cửa sổ nên chi phí tăng tuyến tính theo lưu lượng; ở tải cao vẫn cần bảng rollup theo phút. Đo lại trên dữ liệu thật (đây là số của máy dev, 243k span/ngày). Đánh đổi: span cũ hơn 1 giờ so với cửa sổ không được ghép làm parent/cùng trace. |
| 5 | Secret trong free-text | Đã che pattern phổ biến (`ingest/otlp/scrub.go`, bật mặc định). Văn xuôi như "the password is X" và PII không nhận diện được: lọc thêm ở SDK/Collector, chốt policy. |
| 6 | Incident RESOLVED tái phát | **Đã xử lý:** tự mở lại khi có occurrence mới hơn lúc resolve (cột `actor_key_id` của lịch sử trạng thái cho phép NULL), IGNORED giữ nguyên. Cần theo dõi thực tế xem cửa sổ dedup 1 giờ có phù hợp quy trình on-call không. |
| 7 | Secret mẫu trong production | **Đã xử lý:** `PRODUCTION=true` từ chối khóa bootstrap và mật khẩu DB chứa dấu hiệu mẫu (`development`, `dev-only`, `example`, `changeme`...). |
| 8 | Cổng OTLP sau reverse proxy | **Đã xử lý:** `TRACEPILOT_HTTP_TRUSTED_PROXIES` dùng chung cho REST và OTLP (HTTP và gRPC `x-forwarded-for`); chỉ tin header khi kết nối đến từ proxy đã khai báo. |
| 9 | Cảnh báo cho metrics của TracePilot | Rule mẫu ở mục "Cảnh báo mẫu" bên dưới; cần nạp vào hệ thống giám sát của bạn. |
| 10 | Độ nhạy detection và RCA | **Đã chỉnh:** latency cần ≥ 10 call ở cả hai cửa sổ, throughput cần baseline ≥ 30 call; RCA xếp hạng theo độ tin cậy trước, loại evidence chỉ phân định khi bằng nhau. Ngưỡng vẫn là giá trị khởi điểm, đo lại trên dữ liệu thật. |
| 11 | Index và truy vấn | Trigram GIN của `logs.body` tốn ~20% thông lượng ghi nhưng cần cho tìm nội dung log (đánh đổi do chủ repo chọn). `ListTraces` còn lượt quét thứ hai không có điều kiện thời gian; cần EXPLAIN trên dữ liệu đại diện. |
| 12 | Rate limit nhiều replica | Theo từng tiến trình; đặt ở reverse proxy khi chạy nhiều replica. |
| 13 | Giao diện | Chưa duyệt thủ công: zoom 200%, screen reader, contrast; chưa xem trên dữ liệu thật. |

Giới hạn giữ có chủ đích: hai pipeline có ngân sách byte riêng, không phải giới hạn
tổng RSS (gRPC decode trước admission); một server phục vụ một environment cấu hình;
chỉ metrics Prometheus của chính TracePilot nằm trong bộ nhớ và mất khi tiến trình chết;
RCA là giả thuyết deterministic có evidence, chưa đánh giá trên incident thật.

Sau khi các mục trên có evidence, hướng tiếp theo (chỉ làm khi có số đo): ngưỡng/SLO theo
service, RCA nhiều service với deployment events, tóm tắt bằng LLM tùy chọn không bịa
evidence, và tách tiến trình từ module có bottleneck đã chứng minh. Chưa chọn broker,
graph/vector database hay LLM.

### Cảnh báo mẫu (Prometheus)

Metrics của TracePilot ở `/metrics` (cần key `read` khi `PRODUCTION=true`). Ngưỡng là
điểm khởi đầu, chỉnh theo tải đo được. Chưa có rule nào được nạp sẵn trong repo.

```yaml
groups:
  - name: tracepilot
    rules:
      - alert: TracePilotDatabaseNotReady
        expr: tracepilot_database_ready == 0
        for: 2m
      - alert: TracePilotIngestUnavailable      # exporter nhận 503 và phải gửi lại
        expr: sum(rate(tracepilot_otlp_requests_total{outcome=~"unavailable|overloaded|Unavailable"}[5m])) > 0.5
        for: 5m
      - alert: TracePilotQueueBacklog
        expr: tracepilot_ingestion_queue_depth > 0.8 * 4096   # queue_size mặc định
        for: 5m
      - alert: TracePilotStorageErrors
        expr: increase(tracepilot_event_storage_errors_total[10m]) > 0
      - alert: TracePilotAuthFloodThrottled     # có client sai key liên tục
        expr: sum(rate(tracepilot_otlp_requests_total{outcome=~"throttled|ResourceExhausted"}[5m])) > 0
        for: 10m
      - alert: TracePilotRetentionFailing       # đĩa sẽ đầy dần
        expr: increase(tracepilot_retention_errors_total[2h]) > 0
      - alert: TracePilotRetentionBacklog
        expr: tracepilot_retention_backlog_events > 0
        for: 6h
      - alert: TracePilotDetectionFailing
        expr: increase(tracepilot_incident_detection_runs_total{outcome="error"}[15m]) > 3
      - alert: TracePilotWebhookFailing
        expr: increase(tracepilot_webhook_delivery_attempts_total{outcome=~"failure|claim_error"}[15m]) > 5
```

Cần theo dõi thêm ngoài TracePilot: ngày hết hạn certificate, tuổi bản backup gần nhất,
dung lượng đĩa PostgreSQL và số kết nối. Kiểm tra từng rule bằng cách tạo đúng tình
huống (dừng DB, gửi key sai liên tục) trên staging.

### Đo tải

Dùng `telemetrygen` của OpenTelemetry Collector contrib (không thêm dependency vào repo).
Chạy trên staging hoặc DB dùng một lần, ví dụ 60 giây, 20 worker, 200 span/giây mỗi worker:

```powershell
docker run --rm --network host ghcr.io/open-telemetry/opentelemetry-collector-contrib/telemetrygen:latest `
  traces --otlp-http --otlp-insecure --otlp-endpoint localhost:4318 `
  --otlp-header 'x-api-key="<ingest-key>"' --workers 20 --rate 200 --duration 60s
```

Đối chiếu số span đã lưu (`SELECT count(*) FROM spans WHERE ...`) với số `telemetrygen` báo
đã gửi, ghi p95/p99 từ `tracepilot_otlp_request_duration_seconds`, RSS của container và tăng
trưởng đĩa; thay `logs` hoặc `metrics` cho từng tín hiệu và tăng `--workers` đến khi p99
vượt ngân sách. Phần đó là capacity thật của phần cứng; giới hạn cấu hình thì không phải.
