# Phát triển và kiểm tra

Source map/flow: [ARCHITECTURE.md](ARCHITECTURE.md). Các lệnh chạy từ repo root.
Go lấy version từ backend/go.mod; Node theo web/.nvmrc. Dùng fixture tổng hợp.

## Docker Compose development

```powershell
npm --prefix web ci
npm --prefix web run build
if (-not (Test-Path .env)) { Copy-Item .env.example .env }
# Trong .env: COMPOSE_PROFILES=local, DB host là postgres.
docker compose up -d --wait postgres
docker compose run --build --rm migrate
docker compose up --build -d tracepilot
Invoke-WebRequest http://localhost:8080/health/ready
docker compose run --rm tracepilot mockdata
```

Mở http://localhost:8080 với key development-only:
tracepilot-development-bootstrap-key-000000. Tạo key ingest riêng cho ứng dụng
và key read cho console. Không dùng key demo trong production.

Chỉ có compose.yaml ở root. Profile local bật PostgreSQL 17; migrate là one-shot
chạy chủ động trước application, không tự sửa schema lúc serve. make dev chạy
các bước build UI/start DB/migrate/start app ở trên. Build UI trước rebuild image.
Không overwrite .env đã có. DATABASE_URL trong container dùng hostname postgres;
native dùng localhost. Chỉnh cùng một .env theo cách chạy được chọn.

## Native với PostgreSQL test

```powershell
if (-not (Test-Path .env)) { Copy-Item .env.example .env }
# Điền DB URL dùng hostname localhost và key của bạn trong .env ở root.
go -C backend run ./cmd/tracepilot migrate up
go -C backend run ./cmd/tracepilot mockdata
go -C backend run ./cmd/tracepilot serve
```

Terminal khác:

```powershell
npm --prefix web ci
npm --prefix web run dev
```

Vite proxy API tới :8080. Config chuẩn duy nhất là .env ở root, .env.example chỉ
là mẫu không tự được đọc. Không tạo backend/.env hoặc deploy/.env.
Go đọc .env trong thư mục chạy, thiếu mới tìm ../.env. Vì go -C backend đổi
working directory sang backend/, nó tìm thấy ../.env ở root. Nếu có backend/.env
cũ, file đó sẽ được ưu tiên: chuyển giá trị cần giữ về root rồi xóa bản trùng.
Biến môi trường process ưu tiên hơn file, sau đó tới YAML tùy chọn và defaults.
Compose đọc root .env cho biến nội suy và env_file đưa các giá trị vào container;
override khi chạy container bằng docker compose run -e NAME=value.
CLI serve --config vẫn hỗ trợ YAML version: 1 để tương thích; không cần cho
workflow dùng .env này. Chi tiết chính thức: [Docker env_file](https://docs.docker.com/compose/how-tos/environment-variables/set-environment-variables/).

## Checks

```powershell
go -C backend test ./...
go -C backend vet ./...
go -C backend build ./...
npm --prefix web run check
npm --prefix web run test
npm --prefix web run build
powershell -NoProfile -File scripts/load-test.tests.ps1
```

Linux/macOS dùng pwsh thay powershell. Race detector cần CGO toolchain; CI Linux
chạy go test -race. Unit test không thay thế PostgreSQL integration: khi chưa
bật TRACEPILOT_INTEGRATION_TEST=1, các test DB tự skip.

Thay đổi SQL/schema phải chạy trên DB test có thể bỏ đi. Kiểm tra URL trước:

```powershell
if (-not $env:TRACEPILOT_DISPOSABLE_DATABASE_URL) { throw 'Set disposable DB URL first' }
$env:TRACEPILOT_DATABASE_URL = $env:TRACEPILOT_DISPOSABLE_DATABASE_URL
$env:TRACEPILOT_INTEGRATION_TEST = '1'
go -C backend run ./cmd/tracepilot migrate up
go -C backend test ./...
```

Baseline down/up xóa schema: chỉ dùng scripts/upgrade-rollback-drill.ps1 có
guard trên DB disposable. Không dùng migrate down để rollback application.
Schema hiện là version 2 (000001 baseline, 000002 hàm dependency). Down 1 chỉ
gỡ hàm của 000002; down tiếp mới xóa schema baseline.

Viết SQL mới trong backend/db/queries/<chức năng>.sql (tham số `@tên`, filter
tùy chọn `sqlc.narg(tên)`), sinh lại code rồi gọi hàm sinh ra từ store. Không
truyền chuỗi SQL vào Exec/Query trong Go: architecture test sẽ báo lỗi, trừ các
ngoại lệ có lý do ghi trong storage_sql_test.go. Cấu hình kiểu Go nằm ở
backend/sqlc.yaml. Generated: internal/storage/postgres/sqlc.
Chạy make sqlc-check với Docker, hoặc generator cùng version ghim trong Makefile.
Không sửa generated code bằng tay. Commit UI source và embedded dist tương ứng.
CI kiểm tra SQLC và assets khớp source.

## Công cụ tải

`scripts/loadgen` (Go, chỉ stdlib, nằm ngoài module backend) phát OTLP trace và đọc API
với tốc độ cố định rồi in p50/p95/p99, lỗi theo mã trạng thái và số đọc từ `/metrics`
(queue, pool DB, từ chối). Build và kiểm tra bằng đối số file:

```powershell
go vet scripts/loadgen/main.go scripts/loadgen/report.go
go test scripts/loadgen/main.go scripts/loadgen/report.go scripts/loadgen/report_test.go
go run scripts/loadgen/main.go scripts/loadgen/report.go -help
```

Cách đo và các giới hạn của môi trường ở [OPERATIONS](OPERATIONS.md) và [REVIEW](REVIEW.md).

## Browser smoke

Với DB test đã migrate/seed và server đang chạy:

```powershell
npm --prefix web exec playwright -- install chromium
$env:TRACEPILOT_E2E_BASE_URL = 'http://127.0.0.1:8080'
$env:TRACEPILOT_E2E_API_KEY = 'tracepilot-development-bootstrap-key-000000'
npm --prefix web run test:e2e
```

Chạy binary từ thư mục backend/ (nguồn migration theo cwd) hoặc dùng `go -C backend run`.
Smoke: connect → seeded failed trace → correlated log; `console-surfaces.spec.ts` kiểm tra
theme, Back giữ filter, logs, topology, API key và quyền đọc. CI còn kiểm tra race,
staticcheck, vulnerability, migration roundtrip và image nonroot.
Kết quả thực sự chạy trong phiên này ở [REVIEW.md](REVIEW.md).

## Chọn việc tiếp theo

Trước phạm vi mới, trình bày 2–3 phương pháp, đánh đổi và đề xuất. Khi đã chọn,
hoàn thành callers, tests, config và docs của phạm vi đó. Chỉ thêm dependency,
index hoặc tiến trình khi có nhu cầu/số đo. Các lựa chọn ở [ROADMAP.md](ROADMAP.md).
