# TracePilot

TracePilot là nền tảng quan sát và giám sát hiệu năng ứng dụng (APM & Observability) tự lưu trữ (self-hosted) dành cho các nhóm kỹ thuật. TracePilot tiếp nhận dữ liệu chuẩn OpenTelemetry (OTLP) gồm traces, application logs và metrics; tự động liên kết tương quan giữa sự cố (incidents) và sơ đồ quan hệ phụ thuộc dịch vụ (service topology), đồng thời xếp hạng giả thuyết nguyên nhân gốc rễ (Root Cause Analysis - RCA) với bằng chứng trực quan.

**Trạng thái hiện tại: Beta.** Luồng hoạt động ở [FLOW.md](docs/FLOW.md), sơ đồ ở [diagrams.html](docs/diagrams.html). ACK chỉ trả sau khi kho dữ liệu liên quan xác nhận ghi thành công: PostgreSQL cho quản trị, trace/metric và incident; ClickHouse cho raw log khi được cấu hình.

---

## 🌟 Điểm nổi bật

- **Tương thích toàn diện OpenTelemetry (OTLP):** Hỗ trợ đầy đủ giao thức gRPC (`:4317`) và HTTP/JSON (`:4318`) cho `/v1/traces`, `/v1/logs`, `/v1/metrics`.
- **Kiến trúc gọn:** Một backend Go (API và nhận OTLP), PostgreSQL quản trị/incident/trace/metric, ClickHouse tùy chọn cho raw log, cùng một frontend React riêng. Tái sử dụng pipeline và API hiện có.
- **Giao diện Web Console hiện đại:** Xây dựng với React và Tailwind CSS, hỗ trợ giao diện Sáng/Tối (Dark/Light mode), thanh điều hướng Capsule Dock nổi, bố cục Bento Card trực quan.
- **Trace Explorer & Waterfall:** Xem chi tiết luồng thực thi yêu cầu, phân tích cây span, phát hiện điểm nghẽn độ trễ và lỗi phân tán.
- **Log Correlation:** Tìm kiếm log độc lập theo khung thời gian hoặc truy vết chéo ngay lập tức với trace/span tương ứng.
- **Bản đồ cấu trúc dịch vụ (Topology Graph):** Trực quan hóa tương tác giữa các service nội bộ và hạ tầng bên thứ ba.
- **Phát hiện sự cố & Đề xuất RCA:** Tự động phát hiện lỗi bất thường qua baseline tín hiệu, gom nhóm sự cố và đưa ra giả thuyết nguyên nhân gốc rễ kèm bằng chứng định lượng.
- **Bảo vệ hệ thống & An toàn tài nguyên:** Pipeline tiếp nhận có giới hạn bộ nhớ và hàng đợi, che dữ liệu nhạy cảm; PostgreSQL partition ngày, ClickHouse raw log partition tuần UTC bắt đầu thứ Hai.
- **Tổng hợp nền theo phút:** Dashboard performance và detection service/dependency dùng `span_minute_rollups`, được cập nhật trong worker hiện có. Phần chưa tổng hợp và dữ liệu đến muộn dùng span gốc; p95/p99 vẫn tính từ latency từng request, giữ nguyên API và ranh giới project/environment.

---

## 🚀 Khởi chạy nhanh trên Local

Yêu cầu: một **PostgreSQL có sẵn** (dịch vụ khác), Go theo `backend/go.mod`, và Node theo `frontend/.nvmrc` nếu sửa giao diện.

### 1. Chạy server

```powershell
# 1. Tạo cấu hình rồi sửa TRACEPILOT_DATABASE_URL trỏ tới PostgreSQL của bạn
if (-not (Test-Path backend/.env)) { Copy-Item backend/.env.example backend/.env }

# 2. Tạo schema trong database
go -C backend run ./cmd/tracepilot migrate up

# 3. (Tùy chọn) tạo project mặc định và tài khoản admin development: admin@tracepilot.local / admin-password-123
#    (chỉ dùng ở development; seed bị từ chối khi cấu hình production)
go -C backend run ./cmd/tracepilot seed

# 4. Chạy server
go -C backend run ./cmd/tracepilot serve
```

Backend chỉ phục vụ API; giao diện là ứng dụng riêng ở thư mục `frontend` (xem mục Giao diện bên dưới).

### Raw log trên ClickHouse, PostgreSQL quản trị

Điền `TRACEPILOT_CLICKHOUSE_*` trong **backend/.env** để dùng ClickHouse sẵn có.
URL trống tiếp tục dùng PostgreSQL như trước; URL có giá trị chuyển cả ghi và đọc log
sang ClickHouse. Traces/metrics, tài khoản, project, token, incident và webhook vẫn ở PostgreSQL.
Không tự chuyển về PostgreSQL khi ClickHouse lỗi: ingest trả lỗi tạm để exporter retry.

Khi chuyển một hệ thống đã có log: dừng ingest và chờ pipeline drain, sao lưu/cố định
cấu hình; sau đó chạy lần lượt bằng tài khoản được quyền tạo database/table:

```powershell
go -C backend run ./cmd/tracepilot migrate up
go -C backend run ./cmd/tracepilot migrate clickhouse up
go -C backend run ./cmd/tracepilot migrate clickhouse backfill
go -C backend run ./cmd/tracepilot serve
```

`backfill` phân trang log PostgreSQL theo từng project/environment, giữ nguyên ID và
có thể chạy lại sau khi bị ngắt. Chờ backfill thành công trước khi mở ingest/console.
Log PostgreSQL cũ không bị lệnh backfill xóa; retention PostgreSQL vẫn áp dụng.
Các bản backend cùng phục vụ phải dùng chung cấu hình kho log và thời hạn lưu.
Quay lại URL trống chỉ xem dữ liệu PostgreSQL còn giữ, không tự đồng bộ log mới từ ClickHouse.

`tracepilot_logs` dùng `ReplacingMergeTree`, partition `toStartOfWeek(occurred_at, 1, 'UTC')`.
Ghi batch nén gzip; `async_insert=1, wait_for_async_insert=1` để gộp part phía server
và chỉ xác nhận sau khi flush. Đọc `FINAL` khử trùng theo ID và khóa scope/service/time
kể cả exporter retry với cách chia batch khác. Khóa ORDER BY dùng timestamp cắt xuống
millisecond + ID để backfill microsecond từ PG vẫn khử trùng với retry OTLP nanosecond;
cột timestamp và cursor vẫn giữ nanosecond. Bộ lọc có tham số kiểu rõ ràng,
cursor giữ độ chính xác nanosecond; có index bloom trace/event và ngram cho nội dung log.
Log thường không ghi PostgreSQL; log gây incident chỉ giữ envelope lỗi gọn và vẫn dùng
transaction incident/evidence/outbox cũ. Không có transaction xuyên hai database;
ACK chờ tất cả bước cần thiết và retry có thể lặp lại bước đã thành công.

TTL lấy `TRACEPILOT_RETENTION_MAX_AGE` (mặc định 168h, `0s` giữ mãi), độc lập với partition tuần.
TTL xóa cả part khi mọi dòng trong part hết hạn; API ẩn log hết hạn ngay bằng điều kiện thời gian.
Dữ liệu vật lý có thể lưu thêm gần một tuần cộng độ trễ merge. Đổi thời hạn cần chạy lại
`migrate clickhouse up`; server kiểm tra schema/TTL và từ chối cấu hình lệch, không tự chạy DDL/migration.
Tài khoản runtime cần SELECT/INSERT vào bảng và đọc metadata `system.tables`;
tài khoản migration dùng riêng khi vận hành và không để quyền DDL cho runtime.

Khởi đầu giữ batch 256, 2 worker và 8 kết nối ClickHouse (4 API query, còn chỗ cho ingest/readiness).
Khi tăng số query/worker, tăng connection budget tương ứng để query dài không chiếm hết kết nối ingest.
Khi đo tải, tăng batch lên
1.000–4.000 trước rồi mới tăng worker theo CPU/I/O; giữ queue byte budget, timeout
và giới hạn truy vấn. Theo dõi queue, thời gian persist/retry của TracePilot cùng
`system.parts`, `system.merges`, `system.asynchronous_insert_log`, query latency và disk
trên ClickHouse. Đây là điểm bắt đầu đo tải, không phải cam kết log/giây.
Nếu cần HA, quản trị viên cấp trước bảng `ReplicatedReplacingMergeTree`/ClickHouse Cloud
cùng cột, partition và ORDER BY; patch này không tự tạo shard, replica hay Keeper.

Test tích hợp ClickHouse trên **instance có thể bỏ đi** (tạo/xóa database `tracepilot_test_*`):

```powershell
$env:TRACEPILOT_CLICKHOUSE_INTEGRATION_TEST='1'
$env:TRACEPILOT_CLICKHOUSE_TEST_URL='http://127.0.0.1:18123'
go -C backend test ./internal/storage/clickhouse -count=1
```

Chạy bằng container, PostgreSQL và ClickHouse là dịch vụ ngoài (repo không có Compose):

```powershell
docker build -t tracepilot ./backend
docker run --rm --env-file backend/.env tracepilot migrate up
docker run -d --env-file backend/.env -p 8080:8080 -p 4317:4317 -p 4318:4318 tracepilot
```

Trong container, `localhost` là chính container: hãy dùng tên host hoặc IP mà container tới được trong `TRACEPILOT_DATABASE_URL`.

### 2. Chạy giao diện (Web Console)

Giao diện là ứng dụng riêng, chạy ở một terminal khác (cần Node theo `frontend/.nvmrc`):

```powershell
npm --prefix frontend ci
npm --prefix frontend run dev
```

Mở [http://localhost:5173](http://localhost:5173). Vite tự chuyển các yêu cầu `/api` và `/health` về backend cổng `:8080`, nên không cần cấu hình thêm.

Phải có tài khoản mới vào được console. Khởi tạo tài khoản super admin ban đầu bằng lệnh `tracepilot seed` (chỉ tạo tài khoản super admin development và project mặc định; không nạp telemetry mẫu): đăng nhập bằng `admin@tracepilot.local` / `admin-password-123` (super admin, owner của project mặc định). Các tài khoản khác đăng ký ở tab **Register** (khi `TRACEPILOT_AUTH_ALLOW_REGISTRATION=true`), sau đó super admin sẽ cấp quyền sử dụng: cấp cờ super admin (Settings → Accounts) hoặc phân quyền trong project (Settings → Members).

> [!TIP]
> Sau khi vào console, tạo project cho sản phẩm của bạn (hoặc dùng project mặc định), rồi vào **Settings → Ingest tokens** để tạo token cho ứng dụng gửi dữ liệu.

### Tài khoản, project và phân quyền

Có ba loại người dùng:

| Vai trò | Là ai | Làm được |
| :--- | :--- | :--- |
| **Super admin** | Tài khoản khởi tạo từ seed (`admin@tracepilot.local`), hoặc tài khoản được super admin cấp cờ trong Accounts | Quản lý tài khoản (cấp/hủy admin, khóa, đặt lại mật khẩu), mở mọi project với quyền owner |
| **Owner** | Người tạo project để nhận log của sản phẩm mình | Quản lý project: thành viên, ingest token, environment, webhook, replay, audit |
| **User** | Người được owner thêm vào project | Xem trace, log, incident, topology của project và đổi trạng thái incident |

- Ai đăng ký cũng tạo được project của riêng mình (tối đa 10 project mỗi tài khoản thường) và thành owner của nó. Chưa được owner thêm thì không thấy dữ liệu của project người khác.
- **Đăng nhập dùng JWT:** access token sống 15 phút, console giữ trong bộ nhớ và gửi qua `Authorization: Bearer`. Refresh token ngẫu nhiên nằm trong cookie `HttpOnly`/`SameSite=Strict` chỉ gửi tới `/api/v1/auth/*`, mỗi lần dùng được đổi cái mới (dùng lại cái cũ bị từ chối). Đăng xuất, khóa tài khoản và đổi mật khẩu có hiệu lực ngay, kể cả với access token đang sống.
- **Gửi telemetry vào một project:** owner tạo **ingest token** (Settings → Ingest tokens) — một JWT chỉ ghi được vào project đó, thu hồi được, hết hạn sau 1 năm mặc định. Dùng cho OTLP:
  `OTEL_EXPORTER_OTLP_HEADERS=authorization=Bearer%20<token>`. Chọn environment bằng header `X-Environment` (OTLP gRPC: metadata `x-environment`); không gửi thì dữ liệu vào environment đầu tiên.
- **Không còn API key.** Không có cách đọc dữ liệu bằng khóa tĩnh; công cụ ngoài đăng nhập lấy access token và gửi `X-Project-ID` (ID hoặc slug) cùng `X-Environment` tùy chọn.
- Đặt `TRACEPILOT_AUTH_ALLOW_REGISTRATION=false` để đóng hoàn toàn tính năng đăng ký; khi đó super admin tạo tài khoản trong Settings → Accounts và đưa mật khẩu tạm một lần. Chưa có SSO, MFA hay đặt lại mật khẩu qua email.
- Project mặc định (`TRACEPILOT_INGESTION_PROJECT_*`) vẫn được tạo khi khởi động. Chưa có thao tác xóa project hay environment.

---

## 🛠️ Triển khai giao diện

`npm --prefix frontend run build` tạo thư mục tĩnh `frontend/dist`, đưa lên bất kỳ web server hoặc CDN nào. Có hai cách nối với backend:

- **Cùng origin:** reverse proxy (nginx, Caddy...) chuyển `/api`, `/health` về backend và phục vụ `frontend/dist` cho các đường còn lại (với fallback về `index.html`).
- **Khác origin:** build với `VITE_API_BASE_URL=https://api.example.internal`, và thêm origin của giao diện vào `TRACEPILOT_HTTP_ALLOWED_ORIGINS` của backend.

**Image console (nginx, cùng origin).** [frontend/Dockerfile](frontend/Dockerfile) build giao diện rồi phục vụ bằng nginx nonroot, kèm CSP/`X-Frame-Options` và fallback `index.html`. nginx proxy `/api` tới `TRACEPILOT_API_UPSTREAM` (mặc định `http://tracepilot:8080`, đặt lúc chạy, không cần build lại). Biến của console xem [frontend/.env.example](frontend/.env.example).

```bash
docker build -t tracepilot-console frontend
docker run -d --name tracepilot-console -p 8081:8080 \
  -e TRACEPILOT_API_UPSTREAM=http://<host-backend>:8080 tracepilot-console
```

Backend thấy mọi request qua nginx từ cùng một địa chỉ, nên đặt `TRACEPILOT_HTTP_TRUSTED_PROXIES` là địa chỉ/CIDR của container console để rate limit và log theo đúng IP client. Muốn console khác origin với API, build với `--build-arg VITE_API_BASE_URL=...`.

Header bảo mật của trang (CSP, `X-Frame-Options`) do nơi phục vụ giao diện đặt. Mẫu CSP dùng được: `default-src 'self'; frame-ancestors 'none'; connect-src 'self' <origin-backend>`.

---

## 📡 Cổng giao tiếp & Giao thức

| Dịch vụ | Cổng | Giao thức / Đường dẫn | Mô tả |
| :--- | :--- | :--- | :--- |
| **OTLP gRPC** | `:4317` | gRPC | Tiếp nhận Traces, Logs, Metrics từ OpenTelemetry SDK/Collector |
| **OTLP HTTP** | `:4318` | HTTP/Protobuf hoặc JSON | Endpoints: `/v1/traces`, `/v1/logs`, `/v1/metrics` |
| **Web Console** | `:5173` (dev) | HTTP | Ứng dụng frontend riêng, gọi API qua Vite proxy hoặc `VITE_API_BASE_URL` |
| **Management API**| `:8080` | REST JSON | `/api/v1/...` (xác thực qua HTTP Header `Authorization: Bearer <KEY>`) |
| **Liveness Probe**| `:8080` | HTTP GET | `/health/live` |
| **Readiness Probe**| `:8080` | HTTP GET | `/health/ready` |
| **Internal Metrics**| `:8080` | Prometheus | `/metrics` (đo hiệu năng nội bộ của TracePilot) |

---

## 🏗️ Luồng xử lý & Cấu trúc mã nguồn

```mermaid
flowchart LR
    App["Ứng dụng / OTel SDK"] -->|"OTLP gRPC / HTTP"| Ingest["Bộ thu OTLP (Auth & Sanitize)"]
    Ingest -->|"Batch có kiểm soát bộ nhớ"| Pipeline["Ingest Pipeline & Queue"]
    Pipeline -->|"Trace/metric, incident + outbox"| DB[("PostgreSQL\nPartition ngày")]
    Pipeline -->|"Raw log khi cấu hình URL"| CH[("ClickHouse\nPartition tuần UTC")]
    Pipeline -->|"ACK sau CH flush và PG commit cần thiết"| App
    Detector["Background Detector"] -.->|"Phân tích bất thường"| DB
    Console["React Web Console (ứng dụng riêng)"] <-->|"REST API (:8080)"| HTTPAPI["HTTP API Layer"]
    HTTPAPI <--> DB
    HTTPAPI -->|"Log explorer, correlation"| CH
```

Mã nguồn được tổ chức chặt chẽ theo các ranh giới kiến trúc độc lập (mỗi file tuân thủ giới hạn $\le 300$ dòng):

```text
├── backend/
│   ├── cmd/tracepilot/          # Khởi động CLI, ghép nối dependency, vòng đời tiến trình
│   ├── db/                      # File migrations SQL và query templates (SQLC)
│   └── internal/
│       ├── application/         # Các use case nghiệp vụ (incident, retention, webhook...)
│       ├── domain/              # Model và quy tắc nghiệp vụ thuần túy
│       ├── ingest/              # Bộ chuyển đổi OTLP gRPC/HTTP và pipeline điều tiết
│       ├── storage/             # PostgreSQL, ClickHouse, migration, phân vùng
│       ├── transport/httpapi/   # REST API handler, middleware, rate-limit, OpenAPI
├── frontend/
│   └── src/
│       ├── app/                 # Shell ứng dụng, router, xác thực phiên
│       ├── features/            # Các trang chức năng (overview, traces, logs, topology, settings...)
│       ├── shared/              # Thành phần dùng chung (navigation, paged table, graph layout...)
│       └── styles/              # Chỉ base/theme Tailwind và font tự host
└── docs/                        # FLOW, diagrams.html và bộ kiểm tra Postman
```

---

## ⚙️ Cấu hình môi trường

Cấu hình backend nằm trong một file duy nhất: `backend/.env` (tham khảo mẫu `backend/.env.example`). Console có file riêng, xem `frontend/.env.example`.

Chỉ có **một biến bắt buộc** khi chạy thử là `TRACEPILOT_DATABASE_URL`; mọi biến khác đều có mặc định. Bảng dưới liệt kê toàn bộ 82 tùy chọn; file mẫu `backend/.env.example` chỉ giữ những biến thường chỉnh.

**Quy ước giá trị**
- Thời gian theo cú pháp Go: `500ms`, `30s`, `5m`, `168h`. Danh sách ngăn cách bằng dấu phẩy. Kích thước tính bằng byte.
- Muốn dùng mặc định thì **xóa dòng đó**. Biến có mặt nhưng để rỗng vẫn được áp dụng: kiểu số hoặc thời gian báo lỗi khi khởi động, kiểu chuỗi bị ghi đè bằng chuỗi rỗng.
- Tên biến sai chính tả bị bỏ qua mà không cảnh báo; hãy đối chiếu với bảng.
- Hai ràng buộc hay gặp: `DATABASE_STATEMENT_TIMEOUT` không nhỏ hơn `HTTP_REQUEST_TIMEOUT`, và `HTTP_REQUEST_TIMEOUT` nhỏ hơn `HTTP_WRITE_TIMEOUT`.

**Bắt buộc hoặc thường chỉnh**

| Biến | Mặc định | Ý nghĩa |
| :--- | :--- | :--- |
| `TRACEPILOT_DATABASE_URL` | *(bắt buộc)* | Chuỗi kết nối PostgreSQL của dịch vụ ngoài, ví dụ `postgres://tracepilot:secret@db.internal:5432/tracepilot?sslmode=require`. |
| `TRACEPILOT_AUTH_JWT_SECRET` | *(trống)* | Khóa ký JWT (phiên đăng nhập và ingest token), tối thiểu 32 ký tự. Bắt buộc khi `PRODUCTION=true`, và không được chứa dấu hiệu khóa mẫu. Để trống thì sinh ngẫu nhiên mỗi lần chạy (đăng nhập và token cũ mất hiệu lực khi khởi động lại, các replica không nhận token của nhau). Đổi khóa là thu hồi mọi phiên và token. |
| `TRACEPILOT_AUTH_INGEST_JWT_SECRET` | *(trống)* | Khóa ký JWT riêng cho ingest token OTLP (tối thiểu 32 ký tự). Để trống sẽ tự động dùng chung với `TRACEPILOT_AUTH_JWT_SECRET`. Tách riêng giúp xoay khóa đăng nhập của con người mà không làm gián đoạn ingestion của máy móc. |
| `TRACEPILOT_AUTH_ALLOW_REGISTRATION` | `true` | Cho phép đăng ký tài khoản thông thường. Super admin development được khởi tạo qua seed. |
| `TRACEPILOT_PRODUCTION` | `false` | Bật production: yêu cầu JWT secret, PostgreSQL TLS an toàn và từ chối secret mẫu; TLS của listener tùy chọn. |
| `TRACEPILOT_LOG_LEVEL` | `info` | `debug`, `info`, `warn` hoặc `error`. |
| `TRACEPILOT_SHUTDOWN_TIMEOUT` | `15s` | Thời gian tối đa để flush queue khi tắt. |
| `TRACEPILOT_HTTP_ADDRESS` | `:8080` | Địa chỉ API quản trị, health và metrics. |
| `TRACEPILOT_HTTP_METRICS_TOKEN` | *(trống)* | Token cho Prometheus đọc `/metrics` khi `PRODUCTION=true`, tối thiểu 32 ký tự, gửi bằng `Authorization: Bearer`. Để trống thì chỉ super admin đã đăng nhập đọc được. |
| `TRACEPILOT_HTTP_ALLOWED_ORIGINS` | *(trống)* | Origin của console được gọi API từ trình duyệt (chỉ cần khi console khác origin). |
| `TRACEPILOT_HTTP_TRUSTED_PROXIES` | *(trống)* | CIDR hoặc IP của reverse proxy; để trống thì bỏ qua `X-Forwarded-For`. |
| `TRACEPILOT_TLS_CERTIFICATE_FILE`, `TRACEPILOT_TLS_PRIVATE_KEY_FILE` | *(trống)* | Cặp file TLS cho management/OTLP; tùy chọn, nhưng phải đặt cả hai nếu bật. |
| `TRACEPILOT_RETENTION_MAX_AGE` | `168h` | Thời gian lưu trace, log, metric. `0s` là giữ mãi (áp dụng cho cả bốn biến retention). |
| `TRACEPILOT_RETENTION_INCIDENT_MAX_AGE` | `2160h` | Incident đã đóng. |
| `TRACEPILOT_RETENTION_AUDIT_MAX_AGE` | `4320h` | Lịch sử thay đổi truy cập (tài khoản, project, thành viên, ingest token). |
| `TRACEPILOT_RETENTION_DELIVERY_MAX_AGE` | `720h` | Webhook delivery đã xong. |
| `TRACEPILOT_NOTIFICATIONS_ENABLED` | `false` | Bật gửi webhook; cần `TRACEPILOT_WEBHOOK_ENCRYPTION_KEY`. |
| `TRACEPILOT_WEBHOOK_ENCRYPTION_KEY` | *(trống)* | Khóa 32 byte (hex hoặc base64) mã hóa secret của webhook. |
| `TRACEPILOT_REPLAY_ENABLED` | `false` | Bật replay request; cần `TRACEPILOT_REPLAY_ALLOWED_HOSTS`. |

**HTTP và giới hạn truy vấn**

| Biến | Mặc định | Ý nghĩa |
| :--- | :--- | :--- |
| `TRACEPILOT_HTTP_MAX_CONCURRENT_QUERIES` | `4` | Số request telemetry chạy đồng thời; phần vượt trả 503 (route auth có ngân sách riêng). |
| `TRACEPILOT_HTTP_REQUEST_TIMEOUT` | `20s` | Thời gian tối đa mỗi request; quá hạn trả 504. |
| `TRACEPILOT_HTTP_READ_HEADER_TIMEOUT` / `READ_TIMEOUT` / `WRITE_TIMEOUT` / `IDLE_TIMEOUT` | `5s` / `15s` / `30s` / `1m` | Timeout của HTTP server. |
| `TRACEPILOT_HTTP_MAX_HEADER_BYTES` | `1048576` | Kích thước header tối đa. |
| `TRACEPILOT_HTTP_MANAGEMENT_MAX_BODY_BYTES` | `1048576` | Kích thước body tối đa của API quản trị. |
| `TRACEPILOT_HTTP_RATE_LIMIT_REQUESTS` | `120` | Số request cho mỗi IP trong một cửa sổ. |
| `TRACEPILOT_HTTP_RATE_LIMIT_WINDOW` | `1m` | Độ dài cửa sổ rate limit. |
| `TRACEPILOT_HTTP_RATE_LIMIT_AUTH_REQUESTS` | `20` | Giới hạn riêng cho đăng nhập, đăng ký và refresh (`/api/v1/auth/*`) và ingest token. Đăng nhập còn bị giới hạn 10 lần thử mỗi email mỗi cửa sổ. |
| `TRACEPILOT_HTTP_RATE_LIMIT_REPLAY_REQUESTS` | `10` | Giới hạn riêng cho endpoint `.../replays`. |

**Nhận dữ liệu OTLP (ingestion)**

| Biến | Mặc định | Ý nghĩa |
| :--- | :--- | :--- |
| `TRACEPILOT_INGESTION_GRPC_ADDRESS` / `HTTP_ADDRESS` | `:4317` / `:4318` | Địa chỉ OTLP gRPC và HTTP. Cùng với `TRACEPILOT_HTTP_ADDRESS`, ba địa chỉ phải khác nhau. |
| `TRACEPILOT_INGESTION_PROJECT_ID` / `PROJECT_SLUG` / `PROJECT_NAME` | `01900000-0000-7000-8000-000000000001` / `default` / `Default Project` | Project mặc định, được tạo khi khởi động; tài khoản super admin seed là owner của nó. **Không đổi sau khi đã có dữ liệu**, vì dữ liệu cũ gắn với ID cũ. Lệnh `tracepilot seed` dùng ID và tên trong cấu hình hiện tại. Project khác tạo trong console. |
| `TRACEPILOT_INGESTION_ENVIRONMENT_ID` / `ENVIRONMENT_NAME` | `01900000-0000-7000-8000-000000000002` / `development` | Environment mặc định của project mặc định, cùng lưu ý trên. |
| `TRACEPILOT_INGESTION_MAX_REQUEST_BYTES` | `4194304` | Kích thước request OTLP tối đa (4 MiB). |
| `TRACEPILOT_INGESTION_MAX_RECORDS_PER_REQUEST` | `10000` | Số span/log/metric tối đa mỗi request. |
| `TRACEPILOT_INGESTION_MAX_CONCURRENT_REQUESTS` | `32` | Số request OTLP xử lý đồng thời. |
| `TRACEPILOT_INGESTION_MAX_ATTRIBUTES_PER_RECORD` | `128` | Số attribute tối đa mỗi bản ghi. |
| `TRACEPILOT_INGESTION_MAX_ATTRIBUTE_VALUE_BYTES` | `4096` | Kích thước tối đa một giá trị attribute. |
| `TRACEPILOT_INGESTION_MAX_ATTRIBUTE_DEPTH` | `4` | Độ sâu tối đa của attribute lồng nhau. |
| `TRACEPILOT_INGESTION_QUEUE_SIZE` | `4096` | Số mục tối đa trong queue; phải lớn hơn hoặc bằng `BATCH_SIZE`. |
| `TRACEPILOT_INGESTION_QUEUE_MAX_BYTES` | `67108864` | Ngân sách byte của queue (64 MiB). Không phải giới hạn tổng RSS. |
| `TRACEPILOT_INGESTION_ENQUEUE_TIMEOUT` | `250ms` | Thời gian chờ chỗ trống trong queue trước khi trả 503. |
| `TRACEPILOT_INGESTION_BATCH_SIZE` / `BATCH_TIMEOUT` / `FLUSH_INTERVAL` | `256` / `5s` / `200ms` | Kích thước batch, thời gian tối đa cho mỗi lần ghi batch xuống DB (thử lại tối đa 3 lần) và chu kỳ flush. |
| `TRACEPILOT_INGESTION_WORKERS` | `2` | Số worker ghi DB; `DATABASE_MAX_CONNECTIONS` phải lớn hơn số này ít nhất 2. |
| `TRACEPILOT_INGESTION_REDACTED_KEYS` | *(trống)* | Khóa attribute bổ sung cần che giá trị (không phân biệt hoa thường). |
| `TRACEPILOT_INGESTION_AUTH_FAILURE_LIMIT` / `AUTH_FAILURE_WINDOW` | `60` / `1m` | Sai khóa quá số lần này trong cửa sổ thì bị chặn tạm; `0` là tắt. |
| `TRACEPILOT_INGESTION_SCRUB_FREE_TEXT` | `true` | Che secret trong văn bản tự do. |

**PostgreSQL**

| Biến | Mặc định | Ý nghĩa |
| :--- | :--- | :--- |
| `TRACEPILOT_DATABASE_MAX_CONNECTIONS` / `MIN_CONNECTIONS` | `20` / `2` | Kích thước pool mỗi tiến trình; tổng các replica phải nhỏ hơn `max_connections` của PostgreSQL. |
| `TRACEPILOT_DATABASE_MAX_CONNECTION_LIFETIME` / `MAX_CONNECTION_IDLE_TIME` | `1h` / `30m` | Vòng đời và thời gian rảnh tối đa của một kết nối. |
| `TRACEPILOT_DATABASE_HEALTH_CHECK_PERIOD` / `READINESS_TIMEOUT` | `1m` / `2s` | Chu kỳ kiểm tra pool và thời gian chờ của `/health/ready`. |
| `TRACEPILOT_MIGRATIONS_PATH` | `./db/migrations` | Thư mục migration (image đặt sẵn `/db/migrations`). |
| `TRACEPILOT_DATABASE_STATEMENT_TIMEOUT` / `LOCK_TIMEOUT` / `IDLE_IN_TRANSACTION_TIMEOUT` | `30s` / `10s` / `30s` | Giới hạn phía PostgreSQL; `0` giữ mặc định của server. |

**ClickHouse (raw log tùy chọn)**

| Biến | Mặc định | Ý nghĩa |
| :--- | :--- | :--- |
| `TRACEPILOT_CLICKHOUSE_URL` | *(trống)* | HTTP(S) origin; trống giữ log PostgreSQL. Production yêu cầu HTTPS. |
| `TRACEPILOT_CLICKHOUSE_DATABASE` / `USERNAME` / `PASSWORD` | `tracepilot` / `default` / *(trống)* | Database và tài khoản; không đặt credentials trong URL. |
| `TRACEPILOT_CLICKHOUSE_REQUEST_TIMEOUT` / `MAX_CONNECTIONS` | `10s` / `8` | Deadline HTTP và giới hạn kết nối mỗi backend; ingest còn chịu `INGESTION_BATCH_TIMEOUT`. |

**Retention, replay và webhook (phần còn lại)**

| Biến | Mặc định | Ý nghĩa |
| :--- | :--- | :--- |
| `TRACEPILOT_RETENTION_CHECK_INTERVAL` / `BATCH_SIZE` | `1h` / `10000` | Chu kỳ dọn dẹp và số dòng xóa mỗi lượt. |
| `TRACEPILOT_REPLAY_ALLOWED_HOSTS` | *(trống)* | Danh sách host được phép replay; bắt buộc khi bật replay. |
| `TRACEPILOT_REPLAY_ALLOW_PRIVATE` | `false` | Cho phép replay tới địa chỉ nội bộ. Chỉ bật khi đã chốt allowlist. |
| `TRACEPILOT_REPLAY_TIMEOUT` | `5s` | Thời gian tối đa mỗi lần replay. |
| `TRACEPILOT_REPLAY_MAX_REQUEST_BYTES` / `MAX_RESPONSE_BYTES` | `65536` / `65536` | Giới hạn body gửi đi và nhận về (tối đa 4 MiB mỗi giá trị). |
| `TRACEPILOT_REPLAY_MAX_CONCURRENT` | `2` | Số replay chạy đồng thời (1 đến 16). |
| `TRACEPILOT_WEBHOOK_POLL_INTERVAL` / `WORKERS` / `REQUEST_TIMEOUT` | `5s` / `4` / `10s` | Chu kỳ quét outbox, số worker (1 đến 64) và timeout mỗi lần gửi (tối đa 5 phút). |

Ngoài ra image còn đọc các biến của lệnh `healthcheck` từ môi trường tiến trình (không đọc từ `.env`): `TRACEPILOT_HEALTHCHECK_URL`, `TRACEPILOT_HEALTHCHECK_CA_FILE`, `TRACEPILOT_HEALTHCHECK_SERVER_NAME`. Luồng hoạt động xem [FLOW.md](docs/FLOW.md).

---

## 📚 Tài liệu

| Tài liệu | Nội dung chính |
| :--- | :--- |
| [FLOW.md](docs/FLOW.md) | Mô tả các luồng: khởi động, đăng nhập, phân quyền, ingest, incident, detection, webhook, retention |
| [diagrams.html](docs/diagrams.html) | Sơ đồ hoạt động của các luồng trên (mở bằng trình duyệt) |
| [tracepilot.postman_collection.json](docs/tracepilot.postman_collection.json) | Bộ kiểm tra API import vào Postman (xem mục Phát triển) |

---

## 🧑‍💻 Phát triển

**Cấu hình.** Backend chỉ có `backend/.env` (mẫu `backend/.env.example`); không tạo file riêng cho dev/prod. Go chỉ đọc `backend/.env`, dù chạy từ root repo hay từ thư mục backend; chỉ có một luồng cấu hình: giá trị mặc định, rồi file `.env`, rồi biến môi trường tiến trình (cao nhất). Mọi trường cấu hình ánh xạ sang `TRACEPILOT_<MỤC>_<TRƯỜNG>`. Migration chạy chủ động (`tracepilot migrate up`), server không tự sửa schema khi khởi động.

**Kiểm tra.** Chạy trước khi tích hợp:

```powershell
go -C backend test ./...        # CI Linux chạy thêm -race, staticcheck, govulncheck
go -C backend vet ./...
make sqlc-check                  # sinh lại SQLC và so sánh
npm --prefix frontend run check
npm --prefix frontend run test
npm --prefix frontend run build  # build ra frontend/dist
```

Test cần PostgreSQL tự bỏ qua trừ khi đặt `TRACEPILOT_INTEGRATION_TEST=1` và `TRACEPILOT_DATABASE_URL` trỏ tới **một database dùng một lần**. Thay đổi SQL hoặc migration phải chạy `migrate up`, `migrate down 1`, `migrate up` trên database sạch (hiện có `000001_init`, `000002_identity`, `000003_span_rollups`). Migration 3 lập danh sách phút của spans hiện có; worker backfill tối đa 64 phút/scope mỗi lượt, query đọc dữ liệu gốc trong lúc chờ. Không chạy migrate down trên database thật; rollback image chỉ an toàn khi schema tương thích. Unit test không phải chứng nhận production.

Đo đường đọc gốc và rollup trên cùng 20.000 spans bằng các hàm store hiện có (database dùng một lần đã migrate):

```powershell
$env:TRACEPILOT_INTEGRATION_TEST="1"
go -C backend test -run '^$' -bench SpanRollup -benchtime=1s -benchmem ./internal/storage/postgres
```

**PostgreSQL (SQL-first).** Schema là `backend/db/migrations` (golang-migrate); truy vấn viết trong `backend/db/queries/<chức năng>.sql` (`@tên`, `sqlc.narg(tên)`), `sqlc` kiểm với schema rồi sinh hàm Go có kiểu qua pgx. `make sqlc-check` chạy sinh lại, so sánh với bản đã commit và `sqlc vet` (luật trong `sqlc.yaml`: `UPDATE`/`DELETE` phải có `WHERE`). Không truyền chuỗi SQL vào `Exec/Query` trong Go: architecture test báo lỗi, trừ các ngoại lệ có lý do trong `storage_sql_test.go` (DDL partition cần tên bảng, savepoint, dọn partition mặc định). Bảng của golang-migrate (`schema_migrations`) khai báo riêng cho sqlc ở `db/external/`. Tham số phải đặt tên rõ (`@tên::kiểu`): đừng để tham số nằm trong biểu thức như `@a::timestamptz - interval '1 hour'`, vì sqlc sẽ tự đặt một tham số phụ mà Go không gán; test `TestSqlcParametersAreNamed` bắt lỗi này. Không sửa code sinh ra bằng tay. ClickHouse dùng schema nhúng tại `internal/storage/clickhouse/schema.sql` và HTTP với tham số có kiểu; không đi qua sqlc PostgreSQL.

**Giao diện.** Font Inter và JetBrains Mono được tự host qua `@fontsource-variable` (`frontend/src/styles/fonts.css`, chỉ subset latin, latin-ext, vietnamese); không dùng CDN vì CSP chỉ cho `'self'`. Frontend chỉ còn hai file CSS: `base.css` cho Tailwind, token/theme, reset trong `@layer base`, keyframe và các tùy chọn accessibility; `fonts.css` cho font tự host. Bố cục, responsive, component và SVG dùng Tailwind ngay trong JSX. Các mẫu nút/toolbar dùng lại ở `shared/ui/styles.ts`, form Settings ở `features/settings/form-styles.ts`. Bề mặt glass tự chuyển sang nền đặc khi thiếu blur hoặc người dùng giảm độ trong suốt. Animation đăng nhập dùng CSS/SVG, không thêm thư viện hay vòng lặp JavaScript và tắt khi bật reduced motion.

Style component dùng utility class Tailwind; không thêm CSS riêng theo màn hình hoặc lớp override toàn cục. Dropdown project/environment nổi phía trên nội dung, đồng bộ sáng/tối, có dấu mục đang chọn và hỗ trợ phím mũi tên, Home/End, gõ tên, Enter, Esc và Tab. Scope chỉ đổi khi xác nhận mục; bấm ra ngoài đóng danh sách. Scope nằm trên URL; điều hướng giữ khung thời gian, còn link drill-down `from/to` giữ nguyên khoảng tuyệt đối. Bảng tải thêm chủ động và giới hạn 20 trang/truy vấn; thu hẹp filter để xem tiếp. Waterfall giữ tối đa 5.000 span, render các dòng trong viewport ngay từ đầu và mở inspector riêng để tránh chồng nội dung. Timestamp hiển thị millisecond và timezone của trình duyệt. Count của Overview là số bản ghi đã tải, không phải tổng incident/trace; trạng thái request dựa thêm vào thống kê performance toàn cửa sổ.

Trace detail đặt operation và trạng thái request ở đầu trang, gom thời gian theo service thành dải tóm tắt; service map chỉ mở khi cần. Waterfall hỗ trợ tìm operation/service/span ID và lọc lỗi kể cả trong nhánh đã thu gọn. Chọn span mở inspector bên phải trên desktop, phía dưới trên màn hình nhỏ; các tab hỗ trợ phím mũi tên, Home/End. Log liên quan nằm ngay dưới timeline, thu gọn từng dòng và mở nội dung đầy đủ khi chọn. Ô lọc log chỉ tìm trong các log đã tải; dùng Log explorer để tìm trên server.

Incident investigation có header tóm tắt và các tab Investigation, Occurrences, Linked telemetry, Status history. Trang chỉ tải incident/analysis ban đầu; tab dữ liệu tải khi được mở và dựng một trang 25 dòng. Evidence có số observed/baseline, tham chiếu từ giả thuyết và chi tiết đầy đủ; heuristic score không được trình bày như xác suất root cause.

Live Tail dùng cùng JWT và scope với REST, giữ 100 event mới nhất và khử trùng với log lịch sử. Stream đóng khi JWT hết hạn, kiểm tra lại quyền mỗi 15 giây với deadline 2 giây, mỗi frame có deadline ghi 5 giây. Console tự kết nối lại; 4xx về quyền được hiển thị. Live Tail chỉ nhận broadcast của tiến trình đang giữ kết nối; cache ingest token cũng chưa đảm bảo thu hồi tức thì giữa các replica.

**Postman.** Import `docs/tracepilot.postman_collection.json` (biến `baseUrl`, `otlpUrl` nằm trong file) và chạy thư mục 1 đến 6, hoặc `npx newman run docs/tracepilot.postman_collection.json`. Nó đăng ký tài khoản và project, tạo ingest token, gửi trace/log/metric, xác nhận không credential nào được lưu, và kiểm tra 401/403/413/429, vai trò thành viên, phiên đăng nhập cùng việc token bị thu hồi dừng ngay. Request "Bad token, repeat 65x" chặn cả địa chỉ gọi trong một phút, nên chạy riêng.

Request SSE bỏ qua mặc định để Collection Runner không phải chờ stream dài. Đặt biến collection `runLiveStream=true` khi kiểm tra request này riêng; stream dùng environment `staging` của project được tạo trong collection. Hardening vẫn kiểm tra request stream với chuỗi `session` giả và thiếu project lần lượt trả 401/400.

**Browser.** Với DB đã migrate, tài khoản development được seed và server đang chạy: `npm --prefix frontend exec playwright -- install chromium`, đặt `TRACEPILOT_E2E_BASE_URL` (và tùy chọn `TRACEPILOT_E2E_EMAIL` / `TRACEPILOT_E2E_PASSWORD`, mặc định là tài khoản admin do `tracepilot seed` tạo), rồi `npm --prefix frontend run test:e2e`.

`frontend/e2e/beta-flow.spec.ts` dùng API giả lập, không cần database: chạy console, đặt `TRACEPILOT_E2E_BASE_URL` trỏ tới nó rồi chạy `npm --prefix frontend run test:e2e -- beta-flow.spec.ts`. Các test này kiểm tra scope/thời gian, SSE, waterfall 5.000 span và mobile ở hai theme; không thay thế E2E với backend thật hay đo tải production.

`frontend/e2e/trace-workspace.spec.ts` cũng dùng API giả lập: `npm --prefix frontend run test:e2e -- trace-workspace.spec.ts`. Bộ này kiểm tra trace một span, inspector sáng/tối, lọc lỗi trong cây thu gọn, chuyển tab bằng bàn phím và log dài trên mobile.

`frontend/e2e/incident-workspace.spec.ts` dùng API giả lập để kiểm tra lazy loading theo tab, cursor 25 dòng, đổi trạng thái, giá trị evidence nhỏ và layout sáng/tối/mobile. `beta-flow.spec.ts` còn kiểm tra layout các màn hình chính, form Settings, SVG và đăng nhập khi reduced motion được bật.

---

## 🤝 Đóng góp

Thảo luận thay đổi lớn trong issue trước; sửa nhỏ có thể gửi thẳng pull request. Giữ phạm vi hẹp, thêm test nhỏ nhất bảo vệ hành vi mới, không thêm dependency hạ tầng khi chưa có số đo. Không commit credential hay telemetry thật; chỉ dùng dữ liệu tổng hợp. Mỗi file nguồn giữ dưới 300 dòng (CI kiểm tra).

## 🔒 Bảo mật

Không mở issue công khai cho nghi vấn lỗ hổng liên quan đến lộ credential, truy cập chéo project, SSRF của replay, lạm dụng webhook hay đọc file tùy ý. Dùng tính năng báo cáo lỗ hổng riêng tư của repository, kèm phiên bản hoặc commit, bước tái hiện bằng dữ liệu tổng hợp và tác động quan sát được. Chủ repo cần công bố một đầu mối bảo mật được theo dõi trước bản phát hành đầu tiên. Hiện chỉ bản gắn tag mới nhất và `main` được vá.

Triển khai production phải: bật xác thực và PostgreSQL `verify-full`; nếu management/OTLP đi qua mạng không tin cậy, bật TLS tại listener hoặc kết thúc TLS ở reverse proxy; thay JWT secret mẫu; đặt `TRACEPILOT_AUTH_JWT_SECRET` ngẫu nhiên và giữ trong secret manager, mỗi ứng dụng một ingest token riêng, cấp cho người dùng vai trò user trừ khi họ cần quản lý project, cân nhắc đóng đăng ký, phục vụ console qua HTTPS (cookie refresh `Secure` bắt buộc khi `PRODUCTION=true`); để replay tắt trừ khi đã duyệt allowlist; chỉ dùng webhook HTTPS với encryption key trong secret manager; không ghi token, secret webhook hay telemetry thô vào log. TracePilot chưa được duyệt cho telemetry production cho đến khi chủ repo chấp nhận các tiêu chí do chủ repo chốt (tải, SLO, sao lưu/phục hồi, TLS).

---

## 📄 Bản quyền

Dự án được phân phối dưới giấy phép [MIT](LICENSE).
