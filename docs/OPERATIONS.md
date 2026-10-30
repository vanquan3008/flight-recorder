# Điều kiện vận hành production

Mục tiêu hiện tại: một TracePilot replica, PostgreSQL riêng trên mạng tin cậy,
TLS cho management/OTLP và database, image cố định bằng digest.
Đây là runbook nghiệm thu, không phải tuyên bố source đã được chứng nhận production.
Kết quả kiểm tra ở [REVIEW.md](REVIEW.md); ưu tiên ở [ROADMAP.md](ROADMAP.md).

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
2. Dùng cùng compose.yaml và .env ở root, lấy .env.example làm mẫu. Đặt
   COMPOSE_PROFILES= để tắt PostgreSQL local, TRACEPILOT_PRODUCTION=true,
   TRACEPILOT_IMAGE thành release digest và DATABASE_URL thành DB riêng.
   Dùng bootstrap key riêng đủ mạnh; tạo key ingest/read riêng theo nhiệm vụ.
3. Xem lại bốn giá trị retention (mục Retention) và dung lượng đĩa theo volume thực.
   Mặc định đã bật xóa tự động; 0s trên một nhóm giữ nhóm đó mãi.
4. PostgreSQL dùng sslmode=verify-full và hostname khớp certificate. Với CA riêng,
   mount CA read-only trong container và đặt sslrootcert tới đường dẫn đó.
   Validator hiện vẫn chấp nhận require/verify-ca; đó không thay thế việc chọn
   verify-full cho deployment này.
5. TRACEPILOT_TLS_DIRECTORY chứa tls.crt, tls.key, ca.crt, cho UID nonroot đọc
   được; mount tại /run/tracepilot/tls. Bật các field TLS/healthcheck đã có comment
   trong .env.example. HEALTHCHECK_SERVER_NAME phải khớp DNS certificate. Chỉ mở listener
   tới interface tin cậy; management mặc định bind loopback trên host.
6. Validate Compose, chạy migrate riêng trên DB đã xác minh, rồi start:

```powershell
docker compose config --quiet
docker compose --profile tools pull tracepilot migrate
docker compose run --rm migrate
docker compose up --no-build -d tracepilot
```

Kiểm tra healthcheck container và /health/ready qua HTTPS tin cậy. Query một
trace/log bằng key read; gửi thử bằng key ingest. /metrics yêu cầu quyền read.
Không ghi config có secret, API key hoặc raw telemetry vào log vận hành.
Revoke bootstrap sau khi đã có admin key thay thế; restart cùng bootstrap key
không tự bỏ trạng thái revoke.

env_file: .env chuyển toàn bộ cấu hình từ file root vào container, gồm rate
limit/retention/queue và các field mới. Không dùng --env-file để chọn thêm một
file runtime khác: tham số đó chỉ đổi nguồn nội suy của Compose, env_file trong
service vẫn trỏ .env ở root. Secret manager cần cấp file root này hoặc dùng
environment override rõ ràng. Kiểm tra [Docker env_file](https://docs.docker.com/compose/how-tos/environment-variables/set-environment-variables/).

## Chứng minh dữ liệu và phục hồi

Trước release, chạy trên staging có thể bỏ đi và lưu evidence theo image digest:

| Drill | Điều kiện đạt |
| --- | --- |
| Schema | Up → down 1 → up (000002 gỡ rồi tạo lại ba hàm dependency, dữ liệu giữ nguyên) và integration suite pass trên DB sạch; startup từ chối schema dirty/sai version. Down tiếp xuống baseline xóa schema. |
| Restore | Backup phục hồi vào DB khác; so row counts, scope, sample trace/log/incident/evidence/metrics/outbox; start server trên DB restore rồi kiểm tra readiness/API. |
| Auth/storage outage | Ngắt DB test, OTLP trả retryable, không ACK trước commit; reconnect, retry cùng payload không tăng unique rows. |
| Shutdown | Gửi tải và SIGTERM; ACK chỉ cho dữ liệu commit, không vượt grace, không panic/deadlock; exporter phục hồi phần chưa ACK. |
| Webhook outage | Endpoint test trả 503; retry/outbox tồn tại qua restart, stale sending lease được reclaim, cuối cùng succeed/failed đúng policy. |
| TLS | Certificate chain, DNS/SAN và expiry hợp lệ cho client, healthcheck và PostgreSQL. |

Dùng pg_dump/pg_restore hoặc backup/PITR của managed database; định kỳ thực sự
restore vào DB riêng. Chốt RPO (mức dữ liệu được phép mất) và RTO (thời gian
khôi phục) trước khi triển khai. Chỉ rollback image nếu schema tương thích;
migrate down không phải cách rollback production. Ngoại lệ đã kiểm chứng: image
trước version 2 yêu cầu schema version 1, nên rollback về đó cần `migrate down 1`;
bước này chỉ gỡ ba hàm của 000002, không chạm dữ liệu.

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
đo và giới hạn môi trường ở [REVIEW](REVIEW.md).

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
