# Cấu trúc thư mục TracePilot

TracePilot là một ứng dụng Go (API quản trị, bộ thu OTLP và worker nền chạy chung một tiến trình) cùng một console React.
PostgreSQL lưu dữ liệu quản trị, trace, metric và incident; ClickHouse là kho duy nhất cho raw log.
Luồng hoạt động chi tiết nằm ở [FLOW.md](FLOW.md); cách chạy và phát triển nằm ở [README](../README.md).
Tra cứu file, chữ ký hàm và ví dụ gọi ở [tài liệu HTML](diagrams.html#source-map).

## Mục lục

- [Gốc repo](#gốc-repo)
- [Backend](#backend-backend)
- [Frontend](#frontend-frontend)
- [Build và triển khai](#build-và-triển-khai)
- [Hướng phụ thuộc](#hướng-phụ-thuộc)

## Gốc repo

| Thư mục | Vai trò |
| --- | --- |
| `backend/` | Ứng dụng Go: API quản trị, bộ thu OTLP và các worker nền trong một tiến trình. |
| `frontend/` | Console React (Vite, Tailwind) cho người dùng. |
| `docs/` | `FLOW.md` (mô tả luồng), `diagrams.html` (sơ đồ Mermaid), bộ Postman kiểm tra API và tài liệu này. |
| `.github/` | CI (`ci.yml`), phát hành khi gắn tag (`release.yml`) và action dùng chung `setup-go`. |
| `deploy/` | Triển khai production: `compose.yaml` (migrate chạy một lần, backend, console; database có sẵn bên ngoài) và `nginx.conf.example` cho HTTPS trên host. |
| `docker-compose/` | Chỉ dựng ClickHouse (và giao diện xem dữ liệu) trên máy dev. |

## Backend (`backend/`)

### `cmd/`: chương trình chạy được

| Thư mục | Vai trò |
| --- | --- |
| `cmd/tracepilot/` | Binary chính: `serve`, `migrate`, `create-admin`, `seed`, `healthcheck`, `version`. Đọc cấu hình, nối dependency và quản lý vòng đời tiến trình. |
| `cmd/loadgen/` | Công cụ bắn tải giả lập (OTLP và đọc API) để đo sức chứa. |

### `db/`: SQL embed vào binary

| Thư mục | Vai trò |
| --- | --- |
| `db/postgres/migrations/` | Migration PostgreSQL (`000001_init`, `000002_drop_postgres_logs`). Chỉ thêm file mới, không sửa file đã áp dụng. |
| `db/clickhouse/migrations/` | Migration ClickHouse, dãy version riêng. |
| `db/queries/` | Named query để sqlc sinh code Go. |
| `db/external/` | Khai báo bảng `schema_migrations` của công cụ migration, chỉ để sqlc hiểu schema. |

### `internal/`: code nghiệp vụ theo trách nhiệm

| Thư mục | Vai trò |
| --- | --- |
| `domain/` | Model và quy tắc thuần, không phụ thuộc DB hay HTTP: `telemetry` (event, span, log), `incident` (fingerprint, điều kiện thành incident), `analysis` (RCA, telemetry quality), `notification`, `replay`, `netaddr`. |
| `application/` | Use case và worker nền, gọi tầng dưới qua interface nhỏ. Xem bảng chi tiết bên dưới. |
| `ingest/otlp/` | Nhận OTLP qua HTTP và gRPC; giải mã, chuẩn hóa, che secret. |
| `ingest/pipeline/` | Queue có giới hạn, gom batch theo scope rồi ghi xuống storage. |
| `transport/httpapi/` | REST API `/api/v1`: bảng route, kiểm quyền theo scope, response contract. |
| `storage/postgres/` | Truy cập PostgreSQL. Xem bảng chi tiết bên dưới. |
| `storage/clickhouse/` | Lưu và đọc log, migration và kiểm tra schema của ClickHouse. |
| `storage/migrations/` | Trạng thái migration dùng chung (version, dirty, pending). |
| `auth/` | JWT, ingest token, băm mật khẩu, vai trò và scope, cache xác thực. |
| `config/` | Đọc và kiểm tra cấu hình từ `backend/.env` và biến môi trường. |
| `cache/` | Cache bộ nhớ hoặc Redis, tự quay về bộ nhớ khi Redis lỗi. |
| `query/` | Kiểu dữ liệu đọc dùng chung giữa API và storage (`TraceFilter`, `LogRecord`…). |
| `observability/` | Log JSON nội bộ (`logging`) và Prometheus metrics (`metrics`). |
| `architecture/` | Test bảo vệ ranh giới package và quy tắc SQL/sqlc. |

#### `internal/application/`

| Thư mục | Vai trò |
| --- | --- |
| `detection/` | Mỗi phút so cửa sổ 5 phút với baseline để phát hiện bất thường. |
| `intelligence/` | Chạy detection ngoài luồng ACK của ingest. |
| `incident/` | Đổi trạng thái incident và ghi lịch sử. |
| `live/` | Hub trong bộ nhớ cho Live Tail (SSE). |
| `logs/` | Điều phối ghi log vào ClickHouse và incident vào PostgreSQL, chỉ ACK khi cả hai thành công. |
| `notification/` | Gửi webhook từ outbox, có ký và retry. |
| `replay/` | Gửi lại HTTP request của incident tới host trong allowlist. |
| `retention/` | Dọn dữ liệu hết hạn theo cấu hình. |
| `session/` | Xác minh đăng nhập và đổi mật khẩu. |

#### `internal/transport/httpapi/`

| Thư mục | Vai trò |
| --- | --- |
| `middleware/` | Request ID, security headers, rate limit, CORS, giới hạn body, recover panic. |
| `openapi/` | Sinh spec OpenAPI 3.1 (`GET /openapi.json`) từ bảng route. Mỗi route khai báo tên tham số query/header dùng chung, kiểu Go của request và response (`openapi.Object/Page/List/Empty`); schema sinh bằng reflect theo tag `json`, nên luôn khớp code. Scope ghi trong security requirement của Bearer JWT và `x-required-api-scope`. |
| `render/` | Định dạng JSON và lỗi trả về. |
| `integration/` | Test API với PostgreSQL thật (bật bằng `TRACEPILOT_INTEGRATION_TEST=1`). |

#### `internal/storage/postgres/`

| Thư mục | Vai trò |
| --- | --- |
| `sqlc/` | Code sinh tự động từ `db/queries`; không sửa tay. |
| `identity/` | Tài khoản, phiên đăng nhập, project, environment, thành viên, ingest token. |
| `signals/` | Ghi event và span trong một transaction. |
| `explorer/` | Đọc trace và performance cho console. |
| `analytics/` | Danh sách service, topology, chuỗi metric, tín hiệu cho detection, telemetry quality. |
| `incidents/` | Occurrence, evidence và incident. |
| `outbox/`, `webhooks/` | Hàng đợi giao webhook và cấu hình webhook. |
| `replays/` | Lịch sử các lần replay và kết quả. |
| `maintenance/` | Tạo partition theo ngày và retention. |
| `migrations/` | Runner migration PostgreSQL (advisory lock, status, rollback). |
| `pgbatch/`, `pgvalue/` | Tiện ích gom câu lệnh và chuyển đổi giá trị. |

## Frontend (`frontend/`)

| Thư mục | Vai trò |
| --- | --- |
| `src/app/` | Khung ứng dụng: routing, phiên đăng nhập, header, chọn project và environment, kiểm tra kết nối. |
| `src/features/` | Mỗi thư mục một màn hình (bảng bên dưới). |
| `src/shared/api/` | Client gọi API, refresh token, SSE cho Live Tail. |
| `src/shared/session/` | Kiểu và context đăng nhập. |
| `src/shared/types/` | Kiểu dữ liệu API dùng chung. |
| `src/shared/ui/`, `src/shared/charts/` | Component giao diện và biểu đồ dùng chung. |
| `src/styles/` | CSS và Tailwind. |
| `e2e/` | Test trình duyệt bằng Playwright (smoke journey trong CI). |
| `public/` | File tĩnh. |

| Màn hình (`src/features/`) | Nội dung |
| --- | --- |
| `overview/` | Tổng quan project. |
| `traces/` | Danh sách trace, waterfall, span inspector. |
| `logs/` | Tìm và xem log. |
| `performance/` | Độ trễ và tỉ lệ lỗi theo endpoint. |
| `topology/` | Sơ đồ phụ thuộc giữa các service. |
| `operations/` | Incident, evidence và RCA. |
| `settings/` | Project, thành viên, ingest token, webhook, tài khoản. |
| `onboarding/` | Tạo project và token đầu tiên. |
| `flow/` | Trang giới thiệu luồng sản phẩm. |

## Build và triển khai

| File | Trách nhiệm |
| --- | --- |
| `backend/Dockerfile` | Build native trên host, cross-compile binary tĩnh cho kiến trúc đích; runtime distroless non-root, có readiness healthcheck. |
| `frontend/Dockerfile` | Cache npm, typecheck/build static assets; runtime nginx non-root, có healthcheck `/healthz`. |
| `backend/.dockerignore`, `frontend/.dockerignore` | Thu hẹp build context; không gửi env thật, output và file không cần tới builder. |
| `deploy/compose.yaml` | Production: migration chạy một lần → backend healthy → console. Đọc hai file env hiện có; database bên ngoài, filesystem read-only, port loopback. |
| `deploy/nginx.conf.example` | HTTPS trên host, API/SSE và OTLP/HTTP; cấu hình hostname/chứng chỉ trước khi dùng. |
| `docker-compose/docker-compose.yaml` | Database ClickHouse và UI cho development; không thuộc stack production. |
| `scripts/package-release.ps1` | Build và smoke-test image, đóng gói offline theo đúng layout repo, metadata và checksum. |
| `backend/.env.example` | Mẫu cấu hình backend duy nhất; sao chép thành `backend/.env`. |
| `frontend/.env.example` | Phân biệt biến build `VITE_*` và upstream nginx lúc chạy; sao chép thành `frontend/.env`. |

Compose gán tên container/network theo project; các service liên lạc bằng DNS nội bộ. Network mặc định
`172.30.90.0/24` dùng IP cố định để xác định trusted proxy. Đổi subnet phải đổi đồng thời IP service và
`TRACEPILOT_HTTP_TRUSTED_PROXIES`. Stack buộc production validation và tắt auto-migrate ở `serve`;
chỉ service `migrate` cập nhật schema. Tài liệu vận hành, cập nhật và khôi phục nằm ở
[Production deployment](../README.md#production-deployment).

## Hướng phụ thuộc

- `cmd` nối mọi thứ lại với nhau.
- `transport` và `ingest` gọi `application`.
- `application` dùng `domain` và gọi `storage` qua interface.
- `domain` không phụ thuộc package nào khác trong repo.
- Hai storage adapter (`postgres`, `clickhouse`) không import nhau; `architecture/` kiểm tra điều này.
- Ở frontend, các feature chỉ dùng `shared`, không import ngược vào `app`.
