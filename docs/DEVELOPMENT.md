# Development, test và runbook

Các lệnh dưới đây khớp với binary, Compose và Makefile hiện tại. Lệnh
PowerShell được ưu tiên cho Windows; các lệnh `go -C backend` cũng chạy được
trên Linux/macOS.

## Yêu cầu

- Go `1.26.6` hoặc mới hơn (xem `backend/go.mod`).
- Node.js `24` hoặc mới hơn (xem `web/.nvmrc`) khi sửa operator console React.
- Docker Desktop/Engine và Docker Compose v2.
- PostgreSQL 17 nếu chạy native không dùng Compose.
- `staticcheck` và `govulncheck` chỉ cần khi chạy các gate tương ứng; CI pin
  toolchain trong workflow.

## Chạy local bằng Docker Compose

Lần đầu nên build image và chạy migration tách biệt để thấy lỗi rõ ràng:

```powershell
docker compose -f deploy/compose.dev.yaml up --build -d postgres
docker compose -f deploy/compose.dev.yaml run --build --rm migrate
docker compose -f deploy/compose.dev.yaml up --build -d tracepilot
Invoke-WebRequest http://localhost:8080/health/ready
```

Các cổng mặc định:

| Cổng | Thành phần |
| --- | --- |
| `8080` | management API, UI, health, metrics, OpenAPI |
| `4317` | OTLP gRPC |
| `4318` | OTLP HTTP (`/v1/traces`, `/v1/logs`, `/v1/metrics`; metrics best-effort) |
| `6514` | Syslog TCP/TLS, chỉ hoạt động khi security bật |
| `5432` | PostgreSQL development |

Khi cần reset database development disposable:

```powershell
docker compose -f deploy/compose.dev.yaml down --volumes
```

> Lệnh trên xóa volume PostgreSQL của stack dev. Không dùng với dữ liệu cần
> giữ lại.

## Chạy native server

Khởi động PostgreSQL trước, sau đó dùng host `localhost` thay vì hostname
`postgres` trong Compose:

```powershell
docker compose -f deploy/compose.dev.yaml up -d postgres
$env:TRACEPILOT_DATABASE_URL = "postgres://tracepilot:tracepilot-dev-only@localhost:5432/tracepilot?sslmode=disable"
go -C backend run ./cmd/tracepilot migrate up
go -C backend run ./cmd/tracepilot mockdata
go -C backend run ./cmd/tracepilot serve
```

`mockdata` là tùy chọn và có thể chạy lặp lại. Các subcommand khác:

```powershell
go -C backend run ./cmd/tracepilot version
go -C backend run ./cmd/tracepilot healthcheck
go -C backend run ./cmd/tracepilot migrate version
go -C backend run ./cmd/tracepilot migrate down 1
go -C backend run ./cmd/tracepilot migrate up
```

Server đọc default, YAML (nếu truyền `serve -config path/to/config.yaml` hoặc
`TRACEPILOT_CONFIG_FILE`), rồi environment override. Xem
`backend/internal/config/defaults.go` và `deploy/.env.example` trước khi thêm
biến mới.

## Chạy React operator console

Console source nằm trong `web/`, còn output Vite được commit tại
`backend/internal/webui/dist` để binary Go vẫn nhúng UI qua `go:embed`. Khi
backend đang chạy ở `localhost:8080`, mở terminal khác:

```powershell
npm --prefix web ci
npm --prefix web run dev
```

Vite chạy ở `http://localhost:5173` và proxy `/api` sang backend. Trước khi
commit, chạy:

```powershell
npm --prefix web run check
npm --prefix web run test
npm --prefix web run build
git diff --exit-code -- backend/internal/webui/dist
```

## Chạy file agent

Agent tail file theo newline, export OTLP gRPC, ghi checkpoint và tùy chọn spool
disk khi server tạm mất kết nối:

```powershell
go -C backend run ./cmd/tracepilot-agent `
  -source .\app.log `
  -checkpoint .\var\agent.checkpoint `
  -endpoint localhost:4317 `
  -disk-buffer .\var\agent-spool `
  -api-key-file .\secrets\otlp.key
```

Các flag đáng chú ý: `-batch-bytes` (tối thiểu 64 KiB, mặc định 3 MiB),
`-batch-lines` (mặc định 1000), `-disk-buffer-bytes` (mặc định 64 MiB),
`-tls`, `-tls-server-name`, `-service` và `-metrics-address`. Dùng
`TRACEPILOT_AGENT_API_KEY` hoặc `-api-key-file` thay vì truyền secret trực tiếp
qua process arguments.

## Kiểm tra API nhanh

```powershell
Invoke-WebRequest http://localhost:8080/health/live
Invoke-WebRequest http://localhost:8080/health/ready
Invoke-WebRequest http://localhost:8080/openapi.json
Invoke-WebRequest http://localhost:8080/metrics
```

API v1 cần `Authorization: Bearer <api-key>` (trừ health và môi trường dev khi
metrics chưa yêu cầu auth). Route đầy đủ và scope nằm trong
`/openapi.json`; delivery guarantee và compatibility policy nằm trong
[ARCHITECTURE.md](ARCHITECTURE.md).

## Test và quality gates

Chạy từ thư mục repository:

```powershell
go -C backend test ./...
go -C backend test -race ./...
go -C backend vet ./...
go -C backend test -coverprofile=coverage.out ./...
```

Package test chính:

| Package | Phạm vi |
| --- | --- |
| `internal/domain/*` | invariant, fingerprint, canonical event |
| `internal/ingest/pipeline` | bounded queue, ack, timeout, batch |
| `internal/ingest/otlp` | gRPC/HTTP decode, limit, auth, normalize |
| `internal/ingest/syslog` + `internal/security/parser` | TCP/TLS, line limit, CEF/LEEF/Syslog/JSON |
| `internal/agent` | tail, rotate, checkpoint, spool, exporter |
| `internal/transport/httpapi` | route/OpenAPI parity, auth, pagination, rate limit, integration |
| `internal/storage/postgres` | adapter, migration và PostgreSQL integration |
| `internal/architecture` | dependency direction |

Các gate tùy chọn tương ứng Makefile:

```powershell
go -C backend test -run '^$' -bench '^BenchmarkSubmitBatch$' -benchmem ./internal/ingest/pipeline
$env:TRACEPILOT_INTEGRATION_TEST = "1"
go -C backend test -run '^$' -bench '^BenchmarkStoreEventsIntegration$' -benchtime=5s ./internal/storage/postgres
```

Staticcheck phải chạy trong module:

```powershell
Push-Location backend
go run honnef.co/go/tools/cmd/staticcheck@2026.1 ./...
Pop-Location
```

Govulncheck có thể chạy trực tiếp trong module bằng `go run`, không cần cài
binary global:

```powershell
docker run --rm -v "${PWD}/backend:/src" -w /src golang:1.26.6 `
  sh -c 'go run golang.org/x/vuln/cmd/govulncheck@latest ./...'
```

Race test tương tự trong container Linux (hữu ích khi Windows thiếu cgo):

```powershell
docker run --rm -v "${PWD}/backend:/src" -w /src golang:1.26.6 go test -race ./...
```

## SQLC và migration gate

Sau khi sửa SQL hoặc migration, regenerate bằng image đã pin:

```powershell
docker run --rm -v "${PWD}/backend:/src" -w /src sqlc/sqlc:1.31.1 generate
git diff --exit-code -- backend/internal/storage/postgres/sqlc
```

Lệnh `git diff --exit-code` chỉ có ý nghĩa sau khi generated output đã được
commit hoặc workspace sạch trước khi generate. Server sẽ từ chối start nếu
schema chưa ở migration version hiện tại.

## Integration test với PostgreSQL

```powershell
docker compose -f deploy/compose.dev.yaml up -d postgres
docker compose -f deploy/compose.dev.yaml run --build --rm migrate
$env:TRACEPILOT_INTEGRATION_TEST = "1"
$env:TRACEPILOT_DATABASE_URL = "postgres://tracepilot:tracepilot-dev-only@localhost:5432/tracepilot?sslmode=disable"
go -C backend test -race ./internal/storage/postgres ./internal/transport/httpapi
```

Nếu không set `TRACEPILOT_INTEGRATION_TEST=1`, test cần database sẽ được skip.

## Docker image và production Compose

Build local image:

```powershell
docker build -t tracepilot:dev backend
docker run --rm tracepilot:dev version
```

Image final chạy user non-root, filesystem read-only và chứa migrations cùng hai
binary. Production dùng image digest bất biến và PostgreSQL managed/private:

```powershell
docker compose --env-file C:\secure\tracepilot.env -f deploy/compose.prod.yaml run --rm tracepilot migrate up
docker compose --env-file C:\secure\tracepilot.env -f deploy/compose.prod.yaml up -d
```

File TLS mount phải có `tls.crt`, `tls.key`, `ca.crt`; production validation yêu
cầu Syslog TLS và client CA để bật mTLS khi security được bật. Kiểm tra expiry
và chain trước rotation bằng `.\scripts\cert-check.ps1`.

## Troubleshooting nhanh

- `schema version is not current`: chạy `migrate up` đúng database URL rồi thử
  lại `serve`.
- Kết nối DB native thất bại: dùng `localhost`; hostname `postgres` chỉ tồn tại
  trong network Compose.
- `429 Too Many Requests`: management limiter là fixed-window theo client IP;
  chờ hết `Retry-After` hoặc điều chỉnh các biến `TRACEPILOT_HTTP_RATE_LIMIT_*`.
- Agent không gửi được: kiểm tra endpoint `4317`, API key, `-tls`/server name và
  quyền ghi checkpoint/spool.
- `make` không có trên Windows: chạy các lệnh `go -C backend` và `docker compose`
  ở các mục trên; Makefile chỉ là shortcut.

## Webhook production workflow

Webhook is a v1 feature and is disabled by default. To enable it, set
`TRACEPILOT_NOTIFICATIONS_ENABLED=true` and provide a random 32-byte
`TRACEPILOT_WEBHOOK_ENCRYPTION_KEY` (hex or base64) through a secret manager.
The API is under `/api/v1/webhooks` and requires the `admin` scope:

1. `POST /api/v1/webhooks` creates an HTTPS subscription and returns its signing
   secret once. Store it in the receiving system immediately.
2. `GET` lists metadata only; no endpoint ever returns a stored secret.
3. `POST /api/v1/webhooks/{webhookID}/rotate` invalidates the old secret and
   returns a new one once.
4. `GET /api/v1/webhooks/{webhookID}/deliveries` shows pending, succeeded, and
   terminally failed attempts.

The worker claims rows with PostgreSQL row locks, sends signed JSON with
`X-TracePilot-Signature: sha256=<HMAC-SHA256>`, blocks private/reserved DNS
answers and redirects, and retries pending rows after 30 seconds up to three
attempts. A stale `sending` lease is recovered after one minute.

## Backup, restore, and migration drills

Use a disposable PostgreSQL instance for every drill. The following commands
exercise the same image and migration directory used in production:

```powershell
$env:BACKUP_FILE = Join-Path $PWD "tracepilot-backup.dump"
docker compose -f deploy/compose.dev.yaml exec -T postgres pg_dump -Fc -U tracepilot tracepilot > $env:BACKUP_FILE
docker compose -f deploy/compose.dev.yaml exec -T postgres dropdb -U tracepilot --if-exists tracepilot_restore
docker compose -f deploy/compose.dev.yaml exec -T postgres createdb -U tracepilot tracepilot_restore
Get-Content $env:BACKUP_FILE -AsByteStream | docker compose -f deploy/compose.dev.yaml exec -T postgres pg_restore -U tracepilot -d tracepilot_restore --clean --if-exists
docker compose -f deploy/compose.dev.yaml run --rm -e TRACEPILOT_DATABASE_URL=postgres://tracepilot:tracepilot-dev-only@postgres:5432/tracepilot_restore migrate version
```

For a migration rollback drill, record `migrate version`, run
`migrate down 1`, verify the previous application refuses the schema mismatch,
then run `migrate up` and verify readiness. Never run `down` against the live
database unless the release runbook explicitly names the tested migration.

The repeatable load smoke tests are `make bench-pipeline` and
`make bench-postgres`; run them for at least five minutes in staging and record
p50/p95/p99 latency, ingest rate, queue depth, PostgreSQL WAL, CPU and memory.
Timed soak và query-plan evidence dùng
`.\scripts\load-test.ps1 -DurationMinutes 5 -HealthUrl http://localhost:8080/health/ready -MaxP95Ms 2000`
và `.\scripts\query-plan.ps1`; migration wrapper có tại
`.\scripts\upgrade-rollback-drill.ps1`.
