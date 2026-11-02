# TracePilot

TracePilot là nền tảng quan sát và giám sát hiệu năng ứng dụng (APM & Observability) tự lưu trữ (self-hosted) dành cho các nhóm kỹ thuật. TracePilot tiếp nhận dữ liệu chuẩn OpenTelemetry (OTLP) gồm traces, application logs và metrics; tự động liên kết tương quan giữa sự cố (incidents) và sơ đồ quan hệ phụ thuộc dịch vụ (service topology), đồng thời xếp hạng giả thuyết nguyên nhân gốc rễ (Root Cause Analysis - RCA) với bằng chứng trực quan.

**Trạng thái hiện tại: Beta.** Luồng hoạt động ở [FLOW.md](docs/FLOW.md), sơ đồ ở [diagrams.html](docs/diagrams.html). Dữ liệu traces, logs và OTLP metrics cam kết chỉ phản hồi ACK khi transaction PostgreSQL đã commit thành công.

---

## 🌟 Điểm nổi bật

- **Tương thích toàn diện OpenTelemetry (OTLP):** Hỗ trợ đầy đủ giao thức gRPC (`:4317`) và HTTP/JSON (`:4318`) cho `/v1/traces`, `/v1/logs`, `/v1/metrics`.
- **Kiến trúc gọn:** Một backend Go (API và nhận OTLP) lưu thẳng vào PostgreSQL riêng, cùng một frontend React tách riêng. Không cần thêm middleware phức tạp.
- **Giao diện Web Console hiện đại:** Xây dựng với React và Tailwind CSS, hỗ trợ giao diện Sáng/Tối (Dark/Light mode), thanh điều hướng Capsule Dock nổi, bố cục Bento Card trực quan.
- **Trace Explorer & Waterfall:** Xem chi tiết luồng thực thi yêu cầu, phân tích cây span, phát hiện điểm nghẽn độ trễ và lỗi phân tán.
- **Log Correlation:** Tìm kiếm log độc lập theo khung thời gian hoặc truy vết chéo ngay lập tức với trace/span tương ứng.
- **Bản đồ cấu trúc dịch vụ (Topology Graph):** Trực quan hóa tương tác giữa các service nội bộ và hạ tầng bên thứ ba.
- **Phát hiện sự cố & Đề xuất RCA:** Tự động phát hiện lỗi bất thường qua baseline tín hiệu, gom nhóm sự cố và đưa ra giả thuyết nguyên nhân gốc rễ kèm bằng chứng định lượng.
- **Bảo vệ hệ thống & An toàn tài nguyên:** Pipeline tiếp nhận có giới hạn bộ nhớ (byte budget) và hàng đợi có chặn, tự động loại trừ dữ liệu nhạy cảm (redaction), hỗ trợ phân vùng (partition) bảng theo ngày.

---

## 🚀 Khởi chạy nhanh trên Local

Yêu cầu: một **PostgreSQL có sẵn** (dịch vụ khác), Go theo `backend/go.mod`, và Node theo `frontend/.nvmrc` nếu sửa giao diện.

### 1. Chạy server

```powershell
# 1. Tạo cấu hình rồi sửa TRACEPILOT_DATABASE_URL trỏ tới PostgreSQL của bạn
if (-not (Test-Path backend/.env)) { Copy-Item backend/.env.example backend/.env }

# 2. Tạo schema trong database
go -C backend run ./cmd/tracepilot migrate up

# 3. (Tùy chọn) nạp dữ liệu mẫu và tài khoản admin dev: admin@tracepilot.local / admin-password-123
#    (chỉ trên database dùng để thử; đổi hoặc xóa tài khoản này trước khi dùng thật)
psql $env:TRACEPILOT_DATABASE_URL -f backend/db/seed.sql

# 4. Chạy server
go -C backend run ./cmd/tracepilot serve
```

Backend chỉ phục vụ API; giao diện là ứng dụng riêng ở thư mục `frontend` (xem mục Giao diện bên dưới).

Chạy bằng container (không cần cài Go):

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

Phải có tài khoản mới vào được console. Chọn **Register** để đăng ký bằng email và mật khẩu (tối thiểu 12 ký tự). Nếu đã nạp `seed.sql`, đăng nhập luôn bằng `admin@tracepilot.local` / `admin-password-123` (super admin, owner của project mặc định). Không nạp seed thì **tài khoản đầu tiên** của một bản cài đặt tự thành super admin và là owner của project mặc định; từ đó đăng nhập ở tab **Sign in**.

> [!TIP]
> Sau khi vào console, tạo project cho sản phẩm của bạn (hoặc dùng project mặc định), rồi vào **Settings → Ingest tokens** để tạo token cho ứng dụng gửi dữ liệu.

### Tài khoản, project và phân quyền

Có ba loại người dùng:

| Vai trò | Là ai | Làm được |
| :--- | :--- | :--- |
| **Super admin** | Tài khoản đầu tiên, hoặc tài khoản được super admin khác cấp cờ | Quản lý tài khoản (khóa, đặt lại mật khẩu), mở mọi project với quyền owner |
| **Owner** | Người tạo project để nhận log của sản phẩm mình | Quản lý project: thành viên, ingest token, environment, webhook, replay, audit |
| **User** | Người được owner thêm vào project | Xem trace, log, incident, topology của project và đổi trạng thái incident |

- Ai đăng ký cũng tạo được project của riêng mình (tối đa 10 project mỗi tài khoản thường) và thành owner của nó. Chưa được owner thêm thì không thấy dữ liệu của project người khác.
- **Đăng nhập dùng JWT:** access token sống 15 phút, console giữ trong bộ nhớ và gửi qua `Authorization: Bearer`. Refresh token ngẫu nhiên nằm trong cookie `HttpOnly`/`SameSite=Strict` chỉ gửi tới `/api/v1/auth/*`, mỗi lần dùng được đổi cái mới (dùng lại cái cũ bị từ chối). Đăng xuất, khóa tài khoản và đổi mật khẩu có hiệu lực ngay, kể cả với access token đang sống.
- **Gửi telemetry vào một project:** owner tạo **ingest token** (Settings → Ingest tokens) — một JWT chỉ ghi được vào project đó, thu hồi được, hết hạn sau 1 năm mặc định. Dùng cho OTLP:
  `OTEL_EXPORTER_OTLP_HEADERS=authorization=Bearer%20<token>`. Chọn environment bằng header `X-Environment` (OTLP gRPC: metadata `x-environment`); không gửi thì dữ liệu vào environment đầu tiên.
- **Không còn API key.** Không có cách đọc dữ liệu bằng khóa tĩnh; công cụ ngoài đăng nhập lấy access token và gửi `X-Project-ID` (ID hoặc slug) cùng `X-Environment` tùy chọn.
- Đặt `TRACEPILOT_AUTH_ALLOW_REGISTRATION=false` để đóng đăng ký (tài khoản đầu tiên luôn được phép); khi đó super admin tạo tài khoản và đưa mật khẩu tạm một lần. Chưa có SSO, MFA hay đặt lại mật khẩu qua email.
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
    Pipeline -->|"Transaction"| DB[("PostgreSQL 17\nPartitioned Tables")]
    DB -->|"Commit ACK"| App
    Detector["Background Detector"] -.->|"Phân tích bất thường"| DB
    Console["React Web Console (ứng dụng riêng)"] <-->|"REST API (:8080)"| HTTPAPI["HTTP API Layer"]
    HTTPAPI <--> DB
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
│       ├── storage/             # Tương tác PostgreSQL, migration, phân vùng
│       ├── transport/httpapi/   # REST API handler, middleware, rate-limit, OpenAPI
├── frontend/
│   └── src/
│       ├── app/                 # Shell ứng dụng, router, xác thực phiên
│       ├── features/            # Các trang chức năng (overview, traces, logs, topology, settings...)
│       ├── shared/              # Thành phần dùng chung (navigation, paged table, graph layout...)
│       └── styles/              # Base, components, Tailwind CSS và responsive styles
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
| `TRACEPILOT_AUTH_ALLOW_REGISTRATION` | `true` | Cho phép đăng ký tài khoản. Tài khoản đầu tiên luôn được phép. |
| `TRACEPILOT_PRODUCTION` | `false` | Bật chế độ chạy thật: bắt buộc TLS, `sslmode` an toàn và từ chối secret mẫu. |
| `TRACEPILOT_LOG_LEVEL` | `info` | `debug`, `info`, `warn` hoặc `error`. |
| `TRACEPILOT_SHUTDOWN_TIMEOUT` | `15s` | Thời gian tối đa để flush queue khi tắt. |
| `TRACEPILOT_HTTP_ADDRESS` | `:8080` | Địa chỉ API quản trị, health và metrics. |
| `TRACEPILOT_HTTP_METRICS_TOKEN` | *(trống)* | Token cho Prometheus đọc `/metrics` khi `PRODUCTION=true`, tối thiểu 32 ký tự, gửi bằng `Authorization: Bearer`. Để trống thì chỉ super admin đã đăng nhập đọc được. |
| `TRACEPILOT_HTTP_ALLOWED_ORIGINS` | *(trống)* | Origin của console được gọi API từ trình duyệt (chỉ cần khi console khác origin). |
| `TRACEPILOT_HTTP_TRUSTED_PROXIES` | *(trống)* | CIDR hoặc IP của reverse proxy; để trống thì bỏ qua `X-Forwarded-For`. |
| `TRACEPILOT_TLS_CERTIFICATE_FILE`, `TRACEPILOT_TLS_PRIVATE_KEY_FILE` | *(trống)* | Cặp file TLS, phải đặt cùng nhau; bắt buộc ở production. |
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
| `TRACEPILOT_HTTP_MAX_CONCURRENT_QUERIES` | `4` | Số request quản trị chạy đồng thời; phần vượt trả 503. |
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
| `TRACEPILOT_INGESTION_PROJECT_ID` / `PROJECT_SLUG` / `PROJECT_NAME` | `01900000-0000-7000-8000-000000000001` / `default` / `Default Project` | Project mặc định, được tạo khi khởi động; tài khoản đầu tiên là owner của nó. **Không đổi sau khi đã có dữ liệu**, vì dữ liệu cũ gắn với ID cũ. `seed.sql` dùng đúng cặp ID mặc định. Project khác tạo trong console. |
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

**Cấu hình.** Backend chỉ có `backend/.env` (mẫu `backend/.env.example`); không tạo file riêng cho dev/prod. Go đọc `.env` ở thư mục chạy (`backend/` khi dùng `go -C backend`) rồi `../.env` (tương thích file cũ ở root); chỉ có một luồng cấu hình: giá trị mặc định, rồi file `.env`, rồi biến môi trường tiến trình (cao nhất). Mọi trường cấu hình ánh xạ sang `TRACEPILOT_<MỤC>_<TRƯỜNG>`. Migration chạy chủ động (`tracepilot migrate up`), server không tự sửa schema khi khởi động.

**Kiểm tra.** Chạy trước khi tích hợp:

```powershell
go -C backend test ./...        # CI Linux chạy thêm -race, staticcheck, govulncheck
go -C backend vet ./...
make sqlc-check                  # sinh lại SQLC và so sánh
npm --prefix frontend run check
npm --prefix frontend run test
npm --prefix frontend run build  # build ra frontend/dist
```

Test cần PostgreSQL tự bỏ qua trừ khi đặt `TRACEPILOT_INTEGRATION_TEST=1` và `TRACEPILOT_DATABASE_URL` trỏ tới **một database dùng một lần**. Thay đổi SQL hoặc migration phải chạy `migrate up`, `migrate down 1`, `migrate up` trên database sạch (schema là một migration `000001_init`). Không chạy migrate down trên database thật; rollback image chỉ an toàn khi schema tương thích. Unit test không phải chứng nhận production.

**SQL (SQL-first).** Schema là `backend/db/migrations` (golang-migrate); truy vấn viết trong `backend/db/queries/<chức năng>.sql` (`@tên`, `sqlc.narg(tên)`), `sqlc` kiểm với schema rồi sinh hàm Go có kiểu qua pgx. `make sqlc-check` chạy sinh lại, so sánh với bản đã commit và `sqlc vet` (luật trong `sqlc.yaml`: `UPDATE`/`DELETE` phải có `WHERE`). Không truyền chuỗi SQL vào `Exec/Query` trong Go: architecture test báo lỗi, trừ các ngoại lệ có lý do trong `storage_sql_test.go` (DDL partition cần tên bảng, savepoint, dọn partition mặc định). Bảng của golang-migrate (`schema_migrations`) khai báo riêng cho sqlc ở `db/external/`. Tham số phải đặt tên rõ (`@tên::kiểu`): đừng để tham số nằm trong biểu thức như `@a::timestamptz - interval '1 hour'`, vì sqlc sẽ tự đặt một tham số phụ mà Go không gán; test `TestSqlcParametersAreNamed` bắt lỗi này. Không sửa code sinh ra bằng tay.

**Giao diện.** Font Inter và JetBrains Mono được tự host qua `@fontsource-variable` (`frontend/src/styles/fonts.css`, chỉ subset latin, latin-ext, vietnamese); không dùng CDN vì CSP chỉ cho `'self'`. Kiểu glass: token `--glass*` và `--mesh` ở `base.css`, quy tắc ở `glass.css`; chỉ bề mặt cấp cao nhất làm mờ nền, và tự chuyển sang nền đặc khi thiếu `backdrop-filter` hoặc người dùng bật giảm độ trong suốt.

**Postman.** Import `docs/tracepilot.postman_collection.json` (biến `baseUrl`, `otlpUrl` nằm trong file) và chạy thư mục 1 đến 6, hoặc `npx newman run docs/tracepilot.postman_collection.json`. Nó đăng ký tài khoản và project, tạo ingest token, gửi trace/log/metric, xác nhận không credential nào được lưu, và kiểm tra 401/403/413/429, vai trò thành viên, phiên đăng nhập cùng việc token bị thu hồi dừng ngay. Request "Bad token, repeat 65x" chặn cả địa chỉ gọi trong một phút, nên chạy riêng.

**Browser.** Với DB đã migrate và seed cùng server đang chạy: `npm --prefix frontend exec playwright -- install chromium`, đặt `TRACEPILOT_E2E_BASE_URL` (và tùy chọn `TRACEPILOT_E2E_EMAIL` / `TRACEPILOT_E2E_PASSWORD`, mặc định là tài khoản admin của `seed.sql`), rồi `npm --prefix frontend run test:e2e`.

---

## 🤝 Đóng góp

Thảo luận thay đổi lớn trong issue trước; sửa nhỏ có thể gửi thẳng pull request. Giữ phạm vi hẹp, thêm test nhỏ nhất bảo vệ hành vi mới, không thêm dependency hạ tầng khi chưa có số đo. Không commit credential hay telemetry thật; chỉ dùng dữ liệu tổng hợp. Mỗi file nguồn giữ dưới 300 dòng (CI kiểm tra).

## 🔒 Bảo mật

Không mở issue công khai cho nghi vấn lỗ hổng liên quan đến lộ credential, truy cập chéo project, SSRF của replay, lạm dụng webhook hay đọc file tùy ý. Dùng tính năng báo cáo lỗ hổng riêng tư của repository, kèm phiên bản hoặc commit, bước tái hiện bằng dữ liệu tổng hợp và tác động quan sát được. Chủ repo cần công bố một đầu mối bảo mật được theo dõi trước bản phát hành đầu tiên. Hiện chỉ bản gắn tag mới nhất và `main` được vá.

Triển khai production phải: bật xác thực, TLS cho management/OTLP và PostgreSQL `verify-full`, thay JWT secret mẫu; đặt `TRACEPILOT_AUTH_JWT_SECRET` ngẫu nhiên và giữ trong secret manager, mỗi ứng dụng một ingest token riêng, cấp cho người dùng vai trò user trừ khi họ cần quản lý project, cân nhắc đóng đăng ký, phục vụ console qua HTTPS (cookie refresh `Secure` bắt buộc khi `PRODUCTION=true`); để replay tắt trừ khi đã duyệt allowlist; chỉ dùng webhook HTTPS với encryption key trong secret manager; không ghi token, secret webhook hay telemetry thô vào log. TracePilot chưa được duyệt cho telemetry production cho đến khi chủ repo chấp nhận các tiêu chí do chủ repo chốt (tải, SLO, sao lưu/phục hồi, TLS).

---

## 📄 Bản quyền

Dự án được phân phối dưới giấy phép [MIT](LICENSE).
