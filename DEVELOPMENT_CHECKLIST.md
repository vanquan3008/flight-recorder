# TracePilot Development Checklist

## Mục tiêu v1

Hoàn thiện TracePilot thành backend self-hosted ổn định cho:

- Nhận OpenTelemetry traces và logs qua OTLP/gRPC và OTLP/HTTP.
- Lưu dữ liệu bền vững trong PostgreSQL.
- Tìm kiếm traces, logs, services và incidents qua API.
- Quản lý API keys theo scope.
- Chạy allowed request execution có allowlist và audit đầy đủ.
- Vận hành an toàn bằng Docker, migrations, metrics, backup và restore.

Ngoài phạm vi v1: Kafka, ClickHouse, Kubernetes operator, microservices, custom cache và OTLP metrics.

## Cấu trúc Go production mục tiêu

Go không quy định một project layout chính thức cho mọi ứng dụng. TracePilot dùng cấu trúc tối thiểu phù hợp với một repository chứa hai executable, một Go module và PostgreSQL migrations.

```text
tracepilot/
├── .github/
│   ├── actions/
│   │   └── setup-go/
│   │       └── action.yml
│   ├── workflows/
│   │   ├── ci.yml
│   │   └── release.yml
│   └── dependabot.yml
├── api/
│   └── openapi.yaml
├── backend/
│   ├── cmd/
│   │   ├── tracepilot/
│   │   │   ├── main.go
│   │   │   ├── serve.go
│   │   │   ├── migrate.go
│   │   │   └── healthcheck.go
│   │   └── tracepilot-agent/
│   │       └── main.go
│   ├── internal/
│   │   ├── agent/
│   │   ├── config/
│   │   ├── ingest/
│   │   │   └── otlp/
│   │   ├── management/
│   │   ├── metrics/
│   │   ├── migrations/
│   │   ├── pipeline/
│   │   ├── postgres/
│   │   ├── replay/
│   │   │   └── capture/
│   │   └── telemetry/
│   ├── migrations/
│   │   ├── 000001_core.up.sql
│   │   ├── 000001_core.down.sql
│   │   └── ...
│   ├── .dockerignore
│   ├── Dockerfile
│   ├── go.mod
│   ├── go.sum
│   └── README.md
├── configs/
│   └── tracepilot.example.yaml
├── docs/
│   ├── ARCHITECTURE.md
│   ├── OPERATIONS.md
│   ├── SECURITY.md
│   └── UPGRADING.md
├── scripts/
│   └── compose-smoke.sh
├── .env.example
├── .gitignore
├── compose.yaml
├── CONTRIBUTING.md
├── DEVELOPMENT_CHECKLIST.md
├── LICENSE
├── Makefile
├── README.md
└── SECURITY.md
```

### Trách nhiệm của từng khu vực

#### `backend/cmd/`

- Chứa entrypoint của executable.
- Chỉ parse CLI flags, load config, khởi tạo dependency và quản lý lifecycle.
- Không chứa SQL, OTLP normalization hoặc business rules.
- Mỗi thư mục con tạo đúng một binary.
- `main.go` phải nhỏ; logic kiểm thử được đặt trong hàm hoặc package bên dưới.

#### `backend/internal/telemetry/`

- Chứa normalized event model và validation độc lập hạ tầng.
- Không import HTTP, gRPC, PostgreSQL hoặc package trong `cmd/`.
- Không chứa global mutable state.
- Là model dùng chung giữa ingestion, pipeline và persistence.

#### `backend/internal/config/`

- Chứa defaults, YAML loading, environment overrides và validation.
- Thứ tự ưu tiên: defaults → YAML → environment.
- Từ chối unknown YAML fields và invalid production limits.
- Không kết nối database hoặc khởi động service.
- Không log secret hoặc trả secret trong error message.

#### `backend/internal/ingest/otlp/`

- Chứa OTLP gRPC/HTTP transport, authentication và normalization.
- Validate kích thước request, record count, identifiers và attributes tại trust boundary.
- Redact dữ liệu trước khi đưa vào pipeline.
- Không tự viết SQL.
- Chỉ acknowledge success sau durability boundary đã công bố.

#### `backend/internal/pipeline/`

- Chứa bounded queue, byte budget, batching, retry và graceful drain.
- Không phụ thuộc OTLP protobuf hoặc HTTP.
- Nhận normalized `telemetry.Event` và một hàm persistence nhỏ.
- Có test cho backpressure, timeout, retry và shutdown.

#### `backend/internal/postgres/`

- Chứa PostgreSQL pool, persistence và query implementations.
- Tất cả query phải nhận project/environment scope rõ ràng.
- Dùng parameterized SQL; không ghép input người dùng vào SQL string.
- Transaction boundary nằm tại operation cần atomicity.
- Chuyển lỗi database thành lỗi package có nghĩa khi API cần phân biệt `400`, `404` hoặc `409`.
- Không chứa HTTP response hoặc gRPC status handling.

#### `backend/internal/management/`

- Chứa management HTTP server, routing, middleware và JSON handlers.
- Authenticate và authorize trước khi query dữ liệu.
- Validate input trước khi gọi PostgreSQL.
- Dùng error response schema thống nhất.
- Không chứa SQL trực tiếp.
- Không trả internal error hoặc secret cho client.

#### `backend/internal/replay/`

- Chứa allowed request execution policy và executor.
- `capture/` chỉ sanitize request body và headers.
- Policy chịu trách nhiệm host allowlist, DNS và private-address protection.
- Executor chịu trách nhiệm timeout, concurrency và response-size limit.
- PostgreSQL chỉ lưu execution metadata; không điều khiển outbound HTTP.

#### `backend/internal/agent/`

- Chứa tailer, checkpoint, spool, exporter loop và agent metrics.
- `cmd/tracepilot-agent` chỉ parse flags và quản lý lifecycle.
- File writes phải atomic và dùng permission riêng tư.
- Một file hoặc spool entry lỗi không được làm agent kẹt vô hạn.

#### `backend/internal/metrics/`

- Sở hữu Prometheus registry riêng của ứng dụng.
- Metric labels phải bounded; không dùng project ID, trace ID, URL thô hoặc user input làm label.
- Không chứa business decisions.
- Public/private access của `/metrics` phải được tài liệu hóa.

#### `backend/internal/migrations/` và `backend/migrations/`

- `internal/migrations/` chứa migration runner.
- `migrations/` chứa SQL theo thứ tự version.
- Mỗi migration có file `up` và `down` tương ứng.
- Application startup không tự chạy migration ngầm.
- Binary từ chối readiness nếu schema version không đúng.

#### `api/`, `configs/` và `docs/`

- `api/openapi.yaml` là contract của public management API.
- `configs/tracepilot.example.yaml` chứa mọi cấu hình được hỗ trợ nhưng không chứa secret thật.
- `docs/ARCHITECTURE.md` ghi component boundaries và data flow.
- `docs/OPERATIONS.md` ghi deploy, health, metrics, backup và restore.
- `docs/UPGRADING.md` ghi migration, upgrade và rollback.
- Root `SECURITY.md` dành cho security reporting; `docs/SECURITY.md` dành cho deployment hardening nếu thực sự cần tách.

### Quy tắc dependency

```text
cmd/tracepilot
    └── config + management + ingest/otlp + pipeline + postgres + replay + metrics

cmd/tracepilot-agent
    └── agent

management
    └── config + postgres + replay + metrics

ingest/otlp
    └── config + telemetry + pipeline + postgres principal + metrics

pipeline
    └── telemetry + metrics

postgres
    └── config + telemetry + migrations + replay result model

agent
    └── OTLP client libraries only

telemetry
    └── Go standard library only
```

Quy tắc bắt buộc:

- [ ] Không package nào import `cmd/`.
- [ ] Không tạo import cycle.
- [ ] Domain model không phụ thuộc transport hoặc database.
- [ ] HTTP/gRPC error mapping chỉ nằm ở transport layer.
- [ ] SQL chỉ nằm trong `internal/postgres` hoặc migration files.
- [ ] Không tạo interface chỉ có một implementation nếu test không cần seam đó.
- [ ] Interface nhỏ được khai báo ở package sử dụng nó, không phải package triển khai.
- [ ] Không tạo `pkg/` khi repository chưa có library công khai cho project khác import.
- [ ] Không tạo `utils`, `helpers`, `common` hoặc `base` package chung chung.
- [ ] Shared code chỉ được tách khi có ít nhất hai caller thực tế và cùng semantics.

### Quy ước code production

- [ ] Dùng `context.Context` làm tham số đầu tiên cho I/O operations.
- [ ] Không lưu context trong struct.
- [ ] Mọi goroutine dài hạn có owner, cancellation và shutdown path.
- [ ] Mọi network server có read/write/header/idle timeout phù hợp.
- [ ] Mọi outbound request có timeout.
- [ ] Wrap error bằng `%w` và thêm operation context.
- [ ] Dùng sentinel/typed errors chỉ khi caller cần phân nhánh.
- [ ] Không log rồi trả cùng một lỗi ở mọi layer; log tại boundary có đủ context.
- [ ] Dùng structured logging và không ghi secret/telemetry payload nhạy cảm.
- [ ] Không dùng `panic` cho lỗi runtime có thể xử lý.
- [ ] Không dùng package-level mutable singleton cho database, config hoặc metrics.
- [ ] Validate input tại trust boundary và giữ database constraints làm lớp bảo vệ cuối.
- [ ] Dùng atomic file replacement cho checkpoint và spool metadata.
- [ ] Mọi bounded resource phải có limit cấu hình và metric quan sát.
- [ ] Mọi non-trivial bug fix có test tái hiện lỗi.

### Checklist áp dụng cấu trúc

- [ ] Tạo `api/openapi.yaml` khi bắt đầu Phase 2.
- [ ] Tạo `configs/tracepilot.example.yaml` từ config schema hiện tại.
- [ ] Khôi phục `docs/OPERATIONS.md` trước release v1.
- [ ] Tạo `docs/ARCHITECTURE.md` mô tả đúng cấu trúc trên.
- [ ] Tạo `docs/UPGRADING.md` khi migration contract ổn định.
- [ ] Giữ toàn bộ Go source trong một module `backend/`.
- [ ] Không thêm Go workspace (`go.work`) khi chỉ có một module.
- [ ] Không di chuyển internal packages sang nhiều module.
- [ ] Kiểm tra dependency direction bằng `go list -deps ./...` trong CI nếu cần.
- [ ] Cập nhật cây thư mục này khi public boundary thực sự thay đổi.

## Trạng thái triển khai Phase 0/1

Các mục dưới đây đã được implement và kiểm chứng trong working tree hiện tại:

- [x] Dependabot trỏ tới module Go ở `/backend`.
- [x] Compose smoke dùng project name và PostgreSQL volume riêng cho từng run; cleanup có guard chống nhầm volume dev.
- [x] Có cờ `TRACEPILOT_PRODUCTION`; production bị từ chối nếu thiếu bootstrap API key hoặc TLS certificate.
- [x] Docker/Compose smoke, migration `up/down/up`, `go test`, `go vet`, `go build` và `gofmt` đã chạy thành công.
- [x] Retention chạy hết backlog theo nhiều batch, có timeout cho từng cycle và dừng khi context bị hủy.
- [x] Retention xuất metric số record còn quá hạn sau mỗi cycle.
- [x] Query traces, logs, services, incidents và replay đều lọc theo `project_id` + `environment_id`.
- [x] Mô hình v1 được chốt là một project/environment cho mỗi deployment.
- [x] Có integration coverage cho hai environment trong cùng project và hai project dùng API keys khác nhau.
- [x] Path identifier phổ biến được validate trước khi gọi PostgreSQL.
- [x] Cursor ID được validate theo loại tài nguyên trước khi PostgreSQL cast UUID/trace ID.
- [x] Replay có cleanup execution `started` quá hạn và hoàn tất audit bằng context độc lập có timeout.
- [x] Replay có test timeout, private-DNS rejection và redirect không follow; allowlist chốt theo host, không theo port.
- [x] Replay có integration test client disconnect nhưng execution vẫn được finalize.
- [x] Agent không bị kẹt bởi một dòng vượt giới hạn; dòng lỗi được bỏ qua, checkpoint vẫn tiến và có metric.
- [x] Agent có test truncate/rotation, corrupt checkpoint/spool và disk-buffer full.
- [x] Xóa `payload_artifacts`, `events.payload_ref` và `replay_executions.result_payload_ref` bằng migration có guard dữ liệu.
- [x] Xóa `api_keys.last_used_at` bằng migration có guard dữ liệu vì không có producer/consumer.

Các mục cố ý chưa xóa:

- [x] Giữ legacy spool format `[]string` tạm thời để không làm mất telemetry đang tồn trong buffer cũ; chỉ xóa sau khi có migration/quarantine rõ ràng.
- [x] `last_used_at` của API key đã được xóa; audit lifecycle dùng `api_key_events`.

Các mục còn lại của Phase 0/1 chỉ còn phần Git/release hygiene; các mục runtime chính đã hoàn tất.

## Phase 0 — Ổn định repository

- [x] Stage toàn bộ `backend/`, `.env.example` và `scripts/`.
- [x] Stage việc xóa module Go cũ ở root.
- [x] Kiểm tra Git nhận diện đúng các file rename/move.
- [x] Tạo một commit nguyên tử cho việc đổi cấu trúc repository.
- [x] Sửa `.github/dependabot.yml` để Go module trỏ tới `/backend`.
- [x] Kiểm tra `Makefile` dùng đúng `go -C backend` cho mọi target.
- [x] Kiểm tra Docker build context là `backend/`.
- [x] Kiểm tra release workflow build đúng hai binary.
- [x] Chạy kiểm tra từ một clean clone.

### Hoàn thành khi

- [x] `git status` sạch sau commit.
- [x] `make build-all` chạy thành công từ root.
- [x] `docker build backend` chạy thành công.
- [x] Không còn workflow hoặc script tham chiếu `cmd/flight`, root `go.mod` hoặc root `migrations/`.

## Phase 1 — Sửa release blockers

### 1.1. Smoke test an toàn

- [x] Sinh Compose project name riêng cho mỗi smoke run.
- [x] Sinh PostgreSQL volume name riêng cho mỗi smoke run.
- [x] Không dùng volume dev mặc định trong smoke test.
- [x] Thêm guard từ chối cleanup nếu volume name không có prefix smoke.
- [x] Cleanup container, network và volume sau cả success lẫn failure.
- [x] Thêm timeout tổng cho smoke script.
- [x] Ghi rõ smoke script yêu cầu Bash/Linux hoặc bổ sung bản PowerShell.

### 1.2. Retention

- [x] Lặp xóa nhiều batch cho đến khi số record xóa nhỏ hơn batch size.
- [x] Giới hạn thời gian của mỗi retention cycle.
- [x] Dừng vòng lặp ngay khi context bị hủy.
- [x] Tránh busy loop khi database trả lỗi.
- [x] Thêm metric số record còn quá hạn hoặc tuổi record cũ nhất.
- [x] Thêm test backlog lớn hơn một batch.
- [x] Thêm test retention khi context timeout.
- [x] Kiểm tra cascade delete cho logs, spans và incident links.

### 1.3. Project và environment isolation

- [x] Chốt mô hình v1: một project/environment cho mỗi deployment.
- [x] Thêm `environment_id` vào trace queries.
- [x] Thêm `environment_id` vào log queries.
- [x] Thêm `environment_id` vào service queries.
- [x] Thêm `environment_id` vào incident queries.
- [x] Thêm `environment_id` vào replay execution queries nếu cần.
- [x] Thêm integration test hai environment trong cùng project.
- [x] Thêm integration test hai project dùng API keys khác nhau.
- [x] Xác nhận không query được dữ liệu ngoài scope.

### 1.4. Replay reliability

- [x] Tạo replay execution trước khi gửi request ra ngoài.
- [x] Dùng request context cho outbound request.
- [x] Dùng context độc lập có timeout cho bước hoàn tất audit.
- [x] Luôn ghi trạng thái cuối: `succeeded`, `failed` hoặc `blocked`.
- [x] Thêm cleanup cho execution `started` quá hạn.
- [x] Test client disconnect trong lúc replay.
- [x] Test outbound timeout.
- [x] Test DNS trả private IP.
- [x] Test redirect không được tự động follow.
- [x] Quyết định allowlist theo host hay host và port.

### 1.5. File agent reliability

- [x] Không để oversized line chặn toàn bộ file mãi mãi.
- [x] Chọn và tài liệu hóa hành vi: skip, truncate hoặc quarantine.
- [x] Ghi metric khi bỏ qua dòng lỗi.
- [x] Thêm test file truncate.
- [x] Thêm test file rotation.
- [x] Thêm test corrupt checkpoint.
- [x] Thêm test corrupt spool entry.
- [x] Thêm test disk buffer full.
- [x] Đọc API key từ environment hoặc secret file.
- [x] Không khuyến nghị truyền API key qua command line trong production.

### Hoàn thành khi

- [x] Smoke test không thể xóa volume dev.
- [x] Retention xử lý được backlog nhiều batch.
- [x] Không có cross-project hoặc cross-environment data leak.
- [x] Không còn replay execution kẹt vô hạn ở `started`.
- [x] Một dòng log lỗi không làm agent ngừng tiến về phía trước.

## Phase 2 — Hoàn thiện API v1

### 2.1. Input validation

- [ ] Validate UUID path parameters trước khi gọi PostgreSQL.
- [ ] Validate trace ID gồm 32 ký tự hex.
- [ ] Validate span ID gồm 16 ký tự hex.
- [ ] Validate incident status.
- [ ] Validate API-key scopes.
- [x] Validate pagination cursor.
- [x] Validate `from <= to`.
- [ ] Giới hạn log content search trong time range cho phép.
- [x] Trả `400` cho input sai định dạng.
- [ ] Trả `404` cho resource hợp lệ nhưng không tồn tại.
- [ ] Không để lỗi PostgreSQL syntax/type lộ thành `500` do input người dùng.

### 2.2. API contract

- [ ] Tạo OpenAPI 3.x cho `/api/v1`.
- [ ] Mô tả authentication headers.
- [ ] Mô tả scope cần thiết của từng endpoint.
- [ ] Mô tả pagination và `next_cursor`.
- [ ] Mô tả error response chuẩn.
- [ ] Thêm request/response examples.
- [ ] Thêm contract test cho routes quan trọng.
- [ ] Giữ backward compatibility trong toàn bộ v1.

### 2.3. API keys

- [ ] Test create API key.
- [ ] Test rotate API key.
- [ ] Test revoke API key.
- [ ] Không cho key thiếu `admin` tạo hoặc quản lý key.
- [ ] Không trả key hash qua API.
- [ ] Chỉ trả raw key đúng một lần khi create/rotate.
- [ ] Quyết định cách xử lý self-revoke và last-admin key.
- [x] Quyết định triển khai hoặc xóa `last_used_at`.
- [ ] Thêm endpoint đọc API-key audit events hoặc bỏ schema chưa dùng.

### 2.4. Incident workflow

- [ ] Xác nhận incident fingerprint rules.
- [ ] Xác nhận deduplication window.
- [ ] Test incident được tạo từ HTTP 5xx.
- [ ] Test incident được tạo từ exception.
- [ ] Test incident được tạo từ ERROR/FATAL log.
- [ ] Test duplicate telemetry không tăng occurrence sai.
- [ ] Test status transition hợp lệ.
- [ ] Test status transition không hợp lệ.
- [ ] Quyết định incident đã resolved có tự mở lại khi lỗi tái xuất hiện hay không.

### Hoàn thành khi

- [ ] OpenAPI mô tả đủ toàn bộ public API.
- [ ] Mọi invalid input phổ biến trả đúng `4xx`.
- [ ] API-key scopes được kiểm tra bằng integration tests.
- [ ] Incident lifecycle có hành vi được tài liệu hóa và kiểm thử.

## Phase 3 — Data và migrations

- [ ] Chạy migration `up` trên database sạch.
- [ ] Chạy `down 1` rồi `up` lại.
- [ ] Test startup khi schema chưa migrate.
- [ ] Test startup khi schema dirty.
- [ ] Test startup khi schema cũ hơn binary.
- [ ] Test idempotent ingest cho spans.
- [ ] Test idempotent ingest cho logs.
- [ ] Test concurrent incident deduplication.
- [ ] Kiểm tra index bằng `EXPLAIN ANALYZE` trên dataset đại diện.
- [ ] Benchmark trace list và trace detail.
- [ ] Benchmark log list và content search.
- [ ] Benchmark incident list.
- [x] Quyết định triển khai hoặc xóa `payload_artifacts`.
- [x] Quyết định triển khai hoặc xóa `result_payload_ref`.
- [ ] Không thêm partitioning trước khi benchmark cho thấy cần thiết.

### Hoàn thành khi

- [ ] Migration có đường nâng cấp và rollback đã kiểm thử.
- [ ] Duplicate OTLP delivery không tạo duplicate record.
- [ ] Query chính có latency mục tiêu được công bố.
- [ ] Schema không còn thành phần chưa dùng mà không có quyết định rõ ràng.

## Phase 4 — Testing và CI

### 4.1. Local checks

- [ ] `make fmt-check`.
- [ ] `make vet`.
- [ ] `make staticcheck`.
- [ ] `make vulncheck`.
- [ ] `make test-race` trên Linux/CGO environment.
- [ ] `make build-all`.
- [ ] `make smoke`.

### 4.2. Automated tests

- [ ] Unit test config defaults, env overrides và validation.
- [ ] Unit test OTLP normalization limits và redaction.
- [ ] Unit test queue admission, retries và shutdown.
- [ ] Unit test replay sanitizer và SSRF policy.
- [ ] Integration test PostgreSQL persistence.
- [ ] Integration test management API.
- [ ] Integration test API-key lifecycle.
- [ ] Integration test project/environment isolation.
- [ ] End-to-end test từ OTLP ingest đến API query.
- [ ] End-to-end test incident creation và status update.

### 4.3. CI hardening

- [ ] Pin GitHub Actions bằng commit SHA.
- [ ] Dependabot theo dõi `/backend` Go module.
- [ ] Race test là required check.
- [ ] Vulnerability scan là required check.
- [ ] Integration test là required check.
- [ ] Docker build là required check.
- [ ] Smoke test là required check.
- [ ] Release chỉ chạy sau khi toàn bộ verify jobs thành công.
- [ ] Test đúng release artifact thay vì build lại artifact khác.

### Hoàn thành khi

- [ ] Clean clone chạy toàn bộ CI checks thành công.
- [ ] Không có test phụ thuộc dữ liệu hoặc volume dev.
- [ ] Test failure luôn cleanup tài nguyên tạm.
- [ ] Branch protection yêu cầu CI success trước khi merge.

## Phase 5 — Security hardening

- [ ] Authentication bắt buộc cho production deployment.
- [ ] TLS trực tiếp hoặc TLS termination được tài liệu hóa.
- [ ] Không log raw API key, authorization header hoặc telemetry secret.
- [ ] Redaction áp dụng cho resource, scope, record, span events và links.
- [ ] Test redaction với key viết hoa/thường và separator khác nhau.
- [ ] CORS chỉ cho origin allowlist.
- [ ] Quyết định cách bảo vệ `/metrics`.
- [ ] Giới hạn request body và header.
- [ ] Giới hạn OTLP record count và attribute depth.
- [ ] Giới hạn replay concurrency, timeout và response bytes.
- [ ] Tài liệu hóa API-key rotation.
- [ ] Công bố security contact.
- [ ] Chạy `govulncheck` trong mỗi PR và trước release.
- [ ] Tạo threat model ngắn cho auth, cross-project access, SSRF và arbitrary file access.

### Hoàn thành khi

- [ ] Production config không thể vô tình chạy public với thiết lập development mặc định.
- [ ] Các trust boundary có validation và test.
- [ ] Không có secret trong logs, image layers hoặc release artifacts.
- [ ] Security reporting channel hoạt động.

## Phase 6 — Production operations

### 6.1. Deployment

- [ ] Tách Compose development và production.
- [ ] Không expose PostgreSQL port trong production mặc định.
- [ ] Chạy container bằng non-root user.
- [ ] Chạy filesystem read-only khi có thể.
- [ ] Dùng secret manager hoặc Docker/Kubernetes secret.
- [ ] Tài liệu hóa TLS termination.
- [ ] Tài liệu hóa resource limits.

### 6.2. Backup và recovery

- [ ] Viết PostgreSQL backup procedure.
- [ ] Viết restore procedure.
- [ ] Chạy restore drill trên database disposable.
- [ ] Ghi lại RPO và RTO mục tiêu.
- [ ] Viết migration rollback procedure.
- [ ] Viết disaster recovery checklist.

### 6.3. Monitoring

- [ ] Dashboard HTTP request rate/error/latency.
- [ ] Dashboard OTLP ingest/reject/latency.
- [ ] Dashboard queue depth và inflight bytes.
- [ ] Dashboard batch latency và storage errors.
- [ ] Dashboard database readiness.
- [ ] Dashboard retention throughput và backlog.
- [ ] Alert database unavailable.
- [ ] Alert queue saturation.
- [ ] Alert retention backlog.
- [ ] Alert agent spool gần đầy.
- [ ] Alert replay failure rate.

### 6.4. Capacity validation

- [ ] Xác định ingest throughput mục tiêu.
- [ ] Xác định query latency mục tiêu.
- [ ] Xác định retention period mục tiêu.
- [ ] Load test theo các mục tiêu đã xác định.
- [ ] Soak test tối thiểu 24 giờ.
- [ ] Restart backend trong soak test.
- [ ] Restart PostgreSQL trong soak test.
- [ ] Kiểm tra duplicate và data loss sau test.

### Hoàn thành khi

- [ ] Backup đã được restore thành công.
- [ ] Có dashboard và alerts tối thiểu cho vận hành.
- [ ] Đạt throughput và latency đã công bố.
- [ ] Có runbook cho deploy, upgrade, rollback và incident response.

## Phase 7 — Release v1

- [ ] Chốt semantic version.
- [ ] Chốt changelog và release notes.
- [ ] Build Linux AMD64 binary.
- [ ] Build Linux ARM64 binary.
- [ ] Build file-agent binaries.
- [ ] Build Docker image từ cùng commit.
- [ ] Sinh SHA256 checksums.
- [ ] Sinh SBOM.
- [ ] Ký image và release artifacts.
- [ ] Smoke test release binaries.
- [ ] Smoke test image đã publish.
- [ ] Kiểm tra version, commit và build time metadata.
- [ ] Công bố upgrade notes.
- [ ] Công bố known limitations.

### Hoàn thành khi

- [ ] Release artifact có thể được xác minh và chạy độc lập.
- [ ] Image và binary cùng version, commit và schema version.
- [ ] Release documentation đủ để người mới cài đặt và nâng cấp.

## Phase 8 — Product UI và tính năng sau v1

- [ ] Trace search UI.
- [ ] Trace waterfall.
- [ ] Span detail view.
- [ ] Log explorer.
- [ ] Log-to-trace navigation.
- [ ] Service overview.
- [ ] Service dependency view.
- [ ] Incident list và detail.
- [ ] Incident status workflow.
- [ ] Replay execution history.
- [ ] Saved searches.
- [ ] Dashboards.
- [ ] Alert rules.
- [ ] Email/Slack/webhook notifications.
- [ ] Collector và SDK onboarding examples.
- [ ] Accessibility và responsive layout.

## Definition of Done cho backend v1

- [ ] Repository sạch và build được từ clean clone.
- [ ] Không có release blocker P0 chưa xử lý.
- [ ] `make ci` chạy thành công.
- [ ] Smoke test không ảnh hưởng dữ liệu dev.
- [ ] Không có cross-project hoặc cross-environment data leak.
- [ ] Không mất dữ liệu đã trả success khi shutdown bình thường.
- [ ] Retention bắt kịp ingest throughput đã công bố.
- [ ] Replay execution luôn có audit state cuối.
- [ ] Agent không bị kẹt bởi malformed hoặc oversized log line.
- [ ] OpenAPI, operations và security docs hoàn chỉnh.
- [ ] Backup đã restore thử thành công.
- [ ] Load và soak test đạt mục tiêu.
- [ ] Docker image chạy non-root và có vulnerability scan xanh.
- [ ] Release artifacts có checksum, SBOM và chữ ký.
