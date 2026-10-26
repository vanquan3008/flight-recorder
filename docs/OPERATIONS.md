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
| Retention | max_age và dung lượng đĩa từng nhóm dữ liệu | Retention |
| Tải | Peak, p95/p99, RSS đo trên cardinality đại diện | Tải và giới hạn bộ nhớ |
| Metrics | Chấp nhận best-effort sau ACK hoặc chọn phương pháp bền vững | [ROADMAP](ROADMAP.md) |
| Bảo mật | TLS verify-full, key tối thiểu quyền, bootstrap đã revoke | Preflight bước 4–6 |
| Rollback | Image cũ tương thích schema hiện tại; migrate down không dùng | Chứng minh dữ liệu |

## Preflight và triển khai

1. Xác định có DB cũ không. Source hiện chỉ có baseline schema 1, không nâng cấp
   tại chỗ DB dùng lịch sử migration cũ. Giữ backup và binary tương thích cho DB đó.
2. Dùng cùng compose.yaml và .env ở root, lấy .env.example làm mẫu. Đặt
   COMPOSE_PROFILES= để tắt PostgreSQL local, TRACEPILOT_PRODUCTION=true,
   TRACEPILOT_IMAGE thành release digest và DATABASE_URL thành DB riêng.
   Dùng bootstrap key riêng đủ mạnh; tạo key ingest/read riêng theo nhiệm vụ.
3. Chọn retention max_age và dung lượng đĩa theo volume thực. Giá trị 0s mặc định
   tắt xóa tự động; không được suy ra DB có dung lượng hữu hạn nhờ defaults.
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
| Schema | Baseline up/down/up và integration suite pass trên DB sạch; startup từ chối schema dirty/sai version. Down baseline xóa schema. |
| Restore | Backup phục hồi vào DB khác; so row counts, scope, sample trace/log/incident/evidence/metrics/outbox; start server trên DB restore rồi kiểm tra readiness/API. |
| Auth/storage outage | Ngắt DB test, OTLP trả retryable, không ACK trước commit; reconnect, retry cùng payload không tăng unique rows. |
| Shutdown | Gửi tải và SIGTERM; ACK chỉ cho dữ liệu commit, không vượt grace, không panic/deadlock; exporter phục hồi phần chưa ACK. |
| Webhook outage | Endpoint test trả 503; retry/outbox tồn tại qua restart, stale sending lease được reclaim, cuối cùng succeed/failed đúng policy. |
| TLS | Certificate chain, DNS/SAN và expiry hợp lệ cho client, healthcheck và PostgreSQL. |

Dùng pg_dump/pg_restore hoặc backup/PITR của managed database; định kỳ thực sự
restore vào DB riêng. Chốt RPO (mức dữ liệu được phép mất) và RTO (thời gian
khôi phục) trước khi triển khai. Chỉ rollback image nếu schema tương thích;
migrate down không phải cách rollback production.

Wrapper migration destructive có guard, chỉ dành cho disposable DB:

```powershell
./scripts/upgrade-rollback-drill.ps1 -DatabaseUrl $env:TRACEPILOT_DISPOSABLE_DATABASE_URL -AllowDestructive
```

Không chạy nếu chưa xác minh đúng DB test. Script không tự kiểm chứng tương
thích application N/N-1; cần thử image thật trong staging.

## Tải và giới hạn bộ nhớ

Chốt events/giây, metric points/giây, kích thước payload, concurrent exporters,
cardinality, retention và p95/p99 ACK trước khi đặt SLO. Chạy 2x peak dự kiến,
ghi CPU/RSS, queue, DB pool/WAL, disk/index growth và latency truy vấn.

```powershell
./scripts/load-test.ps1 -Requests 1000 -Concurrency 8 -RecordsPerRequest 20 -ApiKey $env:TRACEPILOT_LOAD_API_KEY -Output ./load-result.json
./scripts/load-test.ps1 -DurationMinutes 5 -Concurrency 8 -MaxP95Ms 2000 -ApiKey $env:TRACEPILOT_LOAD_API_KEY
```

Script này chỉ đo OTLP HTTP JSON logs; 2000 ms là ví dụ threshold, không phải SLO
đã được chứng minh. Mỗi request có record identity riêng. accepted_records là số
được OTLP chấp nhận, không phải phép đếm unique rows trong DB. Đối chiếu rows và
counter storage với expected count trên DB test; partial rejection là failure.
Retry/idempotency cần drill riêng gửi lại chính payload cũ.

Queue byte budget là ước lượng và áp dụng riêng cho events/metrics. Nó không
giới hạn tổng RSS; đặc biệt gRPC decode xảy ra trước admission handler. Metrics
best-effort có thể mất sau ACK khi DB lỗi. Nếu yêu cầu metrics bền vững, chọn
phương pháp trong roadmap trước khi dùng chúng làm nguồn duy nhất cho SLO.

## Retention, thông báo và cảnh báo

Retention telemetry dùng ngày UTC và có thể giữ thêm gần một ngày ở cutoff.
Incident/evidence/audit/delivery history chưa có TTL riêng, vẫn tăng dù telemetry
được dọn. UI có thể gặp evidence tham chiếu trace đã hết hạn.
Chốt policy cho từng nhóm trước khi cam kết thời hạn lưu hoặc dung lượng DB.

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
