# Hướng dẫn đọc source và chạy TracePilot đúng cách

Tài liệu này là điểm bắt đầu để đọc source TracePilot, hiểu luồng runtime và
chạy hệ thống đúng thứ tự.

- [ARCHITECTURE.md](ARCHITECTURE.md): boundary package và contract kỹ thuật.
- [DEVELOPMENT.md](DEVELOPMENT.md): lệnh development, test và troubleshooting.
- [OPERATIONS.md](OPERATIONS.md): production, backup, restore và drill.

## 1. Mô hình tư duy

TracePilot là một modular monolith Go với Console React được nhúng vào binary.
PostgreSQL là durability boundary cho trace và log.

~~~
Application / Collector ─┐
File-log agent ──────────┼─> OTLP receiver ─> validate/redact/normalize
Syslog sender ───────────┘                         │
                                                    v
                                      bounded trace/log pipeline
                                                    │
                                     ACK only after DB commit
                                                    v
                                               PostgreSQL
                                                    │
                           API / embedded Console / detection / webhook outbox
~~~

Ba quy tắc quan trọng khi đọc source:

1. **Trace và log chỉ thành công sau PostgreSQL commit.** Không coi request vào
   queue là đã durable.
2. **Metrics là best-effort.** Metrics đi qua pipeline riêng và có thể bị drop
   khi backpressure hoặc storage lỗi.
3. **cmd là composition root.** Business/domain code không được import
   PostgreSQL, HTTP hoặc config ngược chiều.

## 2. Cây source và trách nhiệm

~~~
backend/
├── cmd/
│   ├── tracepilot/          Server: serve, migrate, mockdata, healthcheck
│   └── tracepilot-agent/    Tail file log và export OTLP gRPC
├── db/
│   ├── migrations/          Lịch sử schema PostgreSQL
│   └── queries/             SQL source cho sqlc
└── internal/
    ├── config/              Default, YAML, environment override, validation
    ├── domain/              Model nghiệp vụ thuần, không I/O
    ├── ingest/
    │   ├── otlp/            HTTP/gRPC receiver, decode và normalize
    │   ├── pipeline/        Queue, batch worker, retry, ACK-after-commit
    │   └── syslog/          Syslog TCP/TLS receiver
    ├── storage/postgres/    Adapter PostgreSQL, writer, query, sqlc output
    ├── transport/httpapi/   HTTP API, auth, middleware, OpenAPI
    ├── security/            Parse CEF/LEEF/Syslog và rule detection
    ├── notification/        Webhook outbox worker
    ├── retention/           Job dọn dữ liệu hết hạn
    └── webui/               Console build được nhúng bằng go:embed

web/src/
├── app/                     App bootstrap, shell, router, auth context
├── features/                Page theo nghiệp vụ: traces, security, operations…
├── shared/                  API client, type, UI primitive, pagination/filter
├── styles/                  CSS base, component và responsive
└── main.tsx                 Điểm vào React
~~~

Khi thay đổi source, đi theo hướng dependency này:

~~~
cmd → ingest / transport → domain / query ← storage/postgres
~~~

Không để domain import transport, config hay storage/postgres. Test
**backend/internal/architecture** bảo vệ quy tắc này.

## 3. Luồng khởi động server

Điểm vào binary là **backend/cmd/tracepilot/main.go**. Các subcommand:

| Command | Mục đích |
| --- | --- |
| serve | Chạy management API, Console, OTLP, Syslog tùy chọn và worker |
| migrate up | Áp toàn bộ migration chưa chạy |
| migrate down 1 | Roll back một migration trên database disposable |
| migrate version | In schema version hiện tại |
| mockdata | Seed fixture development; bị cấm ở production |
| healthcheck | Kiểm tra health endpoint cho container |
| version | In metadata binary |

**cmd/tracepilot/serve.go** là composition root. Thứ tự thực tế:

1. config.Load đọc default → YAML (nếu có) → environment.
2. Kiểm tra migration database đã ở version hiện tại.
3. Mở PostgreSQL pool và khởi tạo project/environment ingestion scope.
4. Tạo bootstrap API key nếu cấu hình có key.
5. Tạo replay service, metrics registry, ingest pipeline và metrics pipeline.
6. Khởi động OTLP gRPC/HTTP, management HTTP, Syslog/detection nếu bật,
   retention nếu được cấu hình và webhook worker nếu bật.
7. Khi nhận SIGINT/SIGTERM, dừng listener rồi drain pipeline trong
   shutdown_timeout.

Nếu server không lên, đọc log theo thứ tự: **config validation → migration
version → PostgreSQL connection → port binding → TLS file**.

## 4. Luồng dữ liệu cần hiểu

### OTLP trace và log

~~~
POST /v1/traces hoặc /v1/logs / OTLP gRPC
  → auth bằng X-API-Key hoặc gRPC metadata
  → giới hạn body/record/attribute
  → normalize thành domain/telemetry.Event
  → redact attribute nhạy cảm
  → pipeline.Submit
  → batch worker
  → postgres.StoreEvents trong transaction
  → response thành công
~~~

Nếu queue đầy hoặc database unavailable, receiver trả response retryable. Đây là
đúng behavior: client cần retry thay vì TracePilot trả success sai.

### Metrics

Metrics đi qua MetricsPipeline, tách khỏi trace/log. Đừng dùng metrics làm
điều kiện xác nhận durability hoặc activation của beta.

### Syslog và security

Khi TRACEPILOT_SECURITY_ENABLED=true, ingest/syslog nhận Syslog TCP/TLS.
Security processor thử nhận dạng Syslog, CEF hoặc LEEF rồi bổ sung field
normalized. Event sau đó đi qua durable pipeline giống log. Detection worker
đọc event đã durable, match rule, tạo detection/incident và enqueue webhook
outbox.

Direct Syslog không có application ACK. Nếu nguồn cần lossless delivery, dùng
sender có disk queue hoặc tracepilot-agent.

### HTTP API và Console

**transport/httpapi/routes.go** là manifest route duy nhất. Manifest này dùng
để đăng ký HTTP router, tính CORS methods, sinh OpenAPI, và ẩn capability tùy
chọn khi subsystem tắt.

Console gọi API qua **web/src/shared/api.ts**, luôn gửi X-API-Key. React Query
quản lý fetch/cache/retry; Console không chứa database logic.

## 5. Chạy development bằng Docker Compose

Đây là đường chạy khuyến nghị vì tái tạo được PostgreSQL, migration và binary.

### Điều kiện

- Docker Desktop/Engine phải đang chạy.
- Docker Compose v2 phải có sẵn.
- Port 8080, 4317, 4318, 5432 chưa bị chiếm.

### Khởi động

~~~powershell
docker compose -f deploy/compose.dev.yaml up --build -d
Invoke-WebRequest http://localhost:8080/health/ready
~~~

Compose bắt đầu postgres, chạy service migrate, rồi chỉ chạy tracepilot khi
migration hoàn thành thành công.

Mở http://localhost:8080 và dùng key **chỉ dành cho máy development**:

~~~text
tracepilot-development-bootstrap-key-000000
~~~

Seed dữ liệu demo để kiểm tra Console end-to-end:

~~~powershell
docker compose -f deploy/compose.dev.yaml run --build --rm tracepilot mockdata
~~~

Sau đó vào Console:

1. Connect API key.
2. Mở **Trace explorer**.
3. Tìm trace 33333333333333333333333333333333.
4. Chọn **Inspect** và kiểm tra correlated log checkout returned 500.

Xem log khi lỗi:

~~~powershell
docker compose -f deploy/compose.dev.yaml logs -f migrate
docker compose -f deploy/compose.dev.yaml logs -f tracepilot
docker compose -f deploy/compose.dev.yaml logs -f postgres
~~~

Dừng stack nhưng giữ database:

~~~powershell
docker compose -f deploy/compose.dev.yaml down
~~~

Chỉ dùng lệnh sau khi muốn xóa database development:

~~~powershell
docker compose -f deploy/compose.dev.yaml down --volumes
~~~

## 6. Chạy native để debug Go

Chỉ chạy PostgreSQL bằng Compose, còn binary chạy trên host:

~~~powershell
docker compose -f deploy/compose.dev.yaml up -d postgres
$env:TRACEPILOT_DATABASE_URL = "postgres://tracepilot:tracepilot-dev-only@localhost:5432/tracepilot?sslmode=disable"
$env:TRACEPILOT_AUTH_BOOTSTRAP_API_KEY = "local-development-key-at-least-32-characters"
go -C backend run ./cmd/tracepilot migrate up
go -C backend run ./cmd/tracepilot serve
~~~

Terminal khác để chạy Console React:

~~~powershell
npm --prefix web ci
npm --prefix web run dev
~~~

Vite chạy tại http://localhost:5173 và proxy API/health về
http://localhost:8080.

## 7. Cấu hình tối thiểu cần nhớ

| Biến môi trường | Khi cần |
| --- | --- |
| TRACEPILOT_DATABASE_URL | Luôn cần nếu không dùng default Compose |
| TRACEPILOT_AUTH_BOOTSTRAP_API_KEY | Bắt buộc ở production; initial principal |
| TRACEPILOT_PRODUCTION=true | Bật validation production |
| TRACEPILOT_HTTP_ALLOWED_ORIGINS | Console/API chạy khác origin |
| TRACEPILOT_SECURITY_ENABLED=true | Bật Syslog/security pipeline |
| TRACEPILOT_NOTIFICATIONS_ENABLED=true | Bật webhook worker |
| TRACEPILOT_WEBHOOK_ENCRYPTION_KEY | Bắt buộc khi webhook bật |

Production bắt buộc API key và TLS. Không copy development bootstrap key hoặc
development PostgreSQL password sang production.

## 8. Cách đọc frontend React

### App layer

- **web/src/main.tsx**: render React root.
- **web/src/app/app.tsx**: tạo QueryClient và HashRouter.
- **web/src/app/app-shell.tsx**: API key session state, connection state,
  navigation và shell.
- **web/src/app/routes.tsx**: mapping URL sang feature page.
- **web/src/app/auth.tsx**: auth context nhỏ.

### Feature layer

Mỗi folder feature chứa UI và data flow cho một use case:

- features/overview: onboarding và activity overview.
- features/traces: search, waterfall, correlated logs.
- features/operations: machine log, incident list/detail/status.
- features/security: event, raw evidence, detection, source inventory.
- features/rules: detection rule CRUD/history.

### Shared layer

- shared/api.ts: fetch wrapper, API error và cursor pagination.
- shared/types.ts: model response API.
- shared/ui.tsx: primitive trình bày dùng lại.
- shared/paged-table.tsx: state loading/error/load-more chung.
- shared/search-params.ts: chuẩn hóa filter form vào URL.

Khi thêm page mới, đặt phần nghiệp vụ vào features/<feature>, không thêm
fetch/query logic vào app. Chỉ đưa code sang shared sau khi ít nhất hai feature
có cùng behavior, không chỉ giống cú pháp.

## 9. Checklist thay đổi an toàn

### Thay đổi backend

1. Xác định domain contract và package owner trước.
2. Nếu đổi schema, thêm migration mới; không sửa migration đã phát hành.
3. Cập nhật db/queries và regenerate SQLC nếu query dùng generator.
4. Thêm test nhỏ cho contract/failure path mới.
5. Chạy:

   ~~~powershell
   go -C backend test ./...
   go -C backend vet ./...
   go -C backend build ./...
   ~~~

### Thay đổi frontend

1. Đặt page/component theo feature.
2. Dùng shared/api.ts thay vì gọi fetch trực tiếp.
3. Giữ filter list trong URL.
4. Có state loading, empty, error/retry cho mọi server query.
5. Sau thay đổi UI:

   ~~~powershell
   npm --prefix web run check
   npm --prefix web run test
   npm --prefix web run build
   git diff --exit-code -- backend/internal/webui/dist
   ~~~

Build web sẽ cập nhật backend/internal/webui/dist; cần commit asset này cùng
source web để Go binary và Docker image phục vụ đúng Console.

### Kiểm tra full local

~~~bash
make web-ci
make test
make vet
make build-all
~~~

make ci chạy thêm SQLC, format, staticcheck, vulnerability scan và race test.

## 10. Lỗi thường gặp

| Triệu chứng | Kiểm tra đầu tiên |
| --- | --- |
| /health/ready không trả 200 | Log migration, database URL, PostgreSQL health và schema version |
| Server thoát ngay khi chạy | Config validation, port trùng, TLS certificate/key path |
| Console báo connection failed | API key, API ở port 8080, browser Network tab, endpoint traces |
| Console build xong nhưng binary UI cũ | Chạy web build và kiểm tra backend/internal/webui/dist |
| OTLP nhận 401 | Header X-API-Key/gRPC metadata, key active, scope ingest |
| OTLP nhận retryable error | Queue đầy hoặc PostgreSQL lỗi; xem metrics, server log và client retry |
| Syslog không vào security event | Security enabled, port/TLS/client CA và raw sender format |
| Webhook không gửi | Notifications enabled, encryption key, URL HTTPS/public, outbox log |

## 11. Production: thứ tự tối thiểu

Không chạy production bằng development Compose. Dùng
[deploy/.env.production.example](../deploy/.env.production.example) làm mẫu,
đặt secret ngoài repository, rồi:

~~~bash
docker compose --env-file /secure/tracepilot.env -f deploy/compose.prod.yaml run --rm tracepilot migrate up
docker compose --env-file /secure/tracepilot.env -f deploy/compose.prod.yaml up -d
~~~

Trước khi nhận traffic thật, hoàn thành backup/restore, upgrade/rollback,
certificate, load và outage drill trong [OPERATIONS.md](OPERATIONS.md).
Production chỉ đạt yêu cầu khi các drill có evidence, không chỉ khi container
đang chạy.
