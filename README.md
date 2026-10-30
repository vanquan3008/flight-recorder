# TracePilot

TracePilot là nền tảng quan sát và giám sát hiệu năng ứng dụng (APM & Observability) tự lưu trữ (self-hosted) dành cho các nhóm kỹ thuật. TracePilot tiếp nhận dữ liệu chuẩn OpenTelemetry (OTLP) gồm traces, application logs và metrics; tự động liên kết tương quan giữa sự cố (incidents) và sơ đồ quan hệ phụ thuộc dịch vụ (service topology), đồng thời xếp hạng giả thuyết nguyên nhân gốc rễ (Root Cause Analysis - RCA) với bằng chứng trực quan.

**Trạng thái hiện tại: Beta.** Quy trình nghiệm thu và danh sách việc còn lại trước production ở [OPERATIONS.md](docs/OPERATIONS.md). Dữ liệu traces, logs và OTLP metrics cam kết chỉ phản hồi ACK khi transaction PostgreSQL đã commit thành công.

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
if (-not (Test-Path .env)) { Copy-Item .env.example .env }

# 2. Tạo schema trong database
go -C backend run ./cmd/tracepilot migrate up

# 3. (Tùy chọn) nạp dữ liệu mẫu để xem thử giao diện (chỉ trên database dùng để thử)
psql $env:TRACEPILOT_DATABASE_URL -f backend/db/seed.sql

# 4. Chạy server
go -C backend run ./cmd/tracepilot serve
```

Backend chỉ phục vụ API; giao diện là ứng dụng riêng ở thư mục `frontend` (xem mục Giao diện bên dưới).

Chạy bằng container (không cần cài Go):

```powershell
docker build -t tracepilot ./backend
docker run --rm --env-file .env tracepilot migrate up
docker run -d --env-file .env -p 8080:8080 -p 4317:4317 -p 4318:4318 tracepilot
```

Trong container, `localhost` là chính container: hãy dùng tên host hoặc IP mà container tới được trong `TRACEPILOT_DATABASE_URL`.

### 2. Chạy giao diện (Web Console)

Giao diện là ứng dụng riêng, chạy ở một terminal khác (cần Node theo `frontend/.nvmrc`):

```powershell
npm --prefix frontend ci
npm --prefix frontend run dev
```

Mở [http://localhost:5173](http://localhost:5173). Vite tự chuyển các yêu cầu `/api` và `/health` về backend cổng `:8080`, nên không cần cấu hình thêm.

Nhập mã khóa khởi tạo dành riêng cho môi trường phát triển (bootstrap key):
```text
tracepilot-development-bootstrap-key-000000
```

> [!TIP]
> Sau khi vào console, hãy vào mục **Settings** để tạo Ingestion API Key riêng cho các ứng dụng cần gửi dữ liệu, và Read API Key cho việc truy vấn hàng ngày.

---

## 🛠️ Triển khai giao diện

`npm --prefix frontend run build` tạo thư mục tĩnh `frontend/dist`, đưa lên bất kỳ web server hoặc CDN nào. Có hai cách nối với backend:

- **Cùng origin:** reverse proxy (nginx, Caddy...) chuyển `/api`, `/health` về backend và phục vụ `frontend/dist` cho các đường còn lại (với fallback về `index.html`).
- **Khác origin:** build với `VITE_API_BASE_URL=https://api.example.internal`, và thêm origin của giao diện vào `TRACEPILOT_HTTP_ALLOWED_ORIGINS` của backend.

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
└── docs/                        # ARCHITECTURE, OPERATIONS và bộ kiểm tra Postman
```

---

## ⚙️ Cấu hình môi trường

Tất cả cấu hình hệ thống được định nghĩa thống nhất tại một file độc lập duy nhất: `.env` ở thư mục gốc (tham khảo mẫu `.env.example`).

Các biến cốt lõi:
- `TRACEPILOT_DATABASE_URL`: Chuỗi kết nối PostgreSQL của dịch vụ ngoài (ví dụ `postgres://tracepilot:secret@db.internal:5432/tracepilot?sslmode=require`).
- `TRACEPILOT_AUTH_BOOTSTRAP_API_KEY`: Khóa quản trị khởi tạo (tối thiểu 32 ký tự; production phải đổi khác khóa mẫu).
- `TRACEPILOT_RETENTION_MAX_AGE` (`168h`), `..._INCIDENT_MAX_AGE` (`2160h`), `..._AUDIT_MAX_AGE` (`4320h`), `..._DELIVERY_MAX_AGE` (`720h`): thời gian lưu từng nhóm dữ liệu, `0s` là giữ mãi.
- `TRACEPILOT_INGESTION_AUTH_FAILURE_LIMIT`, `TRACEPILOT_INGESTION_SCRUB_FREE_TEXT`, `TRACEPILOT_DATABASE_STATEMENT_TIMEOUT`: giới hạn xác thực OTLP, che secret trong văn bản tự do và timeout kết nối DB (giá trị mặc định và giải thích trong `.env.example`).

---

## 📚 Tài liệu

| Tài liệu | Nội dung chính |
| :--- | :--- |
| [ARCHITECTURE.md](docs/ARCHITECTURE.md) | Kiến trúc, trách nhiệm từng package, luồng dữ liệu, schema và vận hành pipeline |
| [OPERATIONS.md](docs/OPERATIONS.md) | Triển khai production, TLS, sao lưu/phục hồi, retention, và **việc còn lại trước production** |
| [tracepilot.postman_collection.json](docs/tracepilot.postman_collection.json) | Bộ kiểm tra API import vào Postman (xem mục Phát triển) |

---

## 🧑‍💻 Phát triển

**Cấu hình.** Chỉ có `.env` ở thư mục gốc (mẫu `.env.example`); không tạo `backend/.env` hay file riêng cho dev/prod. Go đọc `.env` ở thư mục chạy rồi `../.env`; chỉ có một luồng cấu hình: giá trị mặc định, rồi file `.env`, rồi biến môi trường tiến trình (cao nhất). Mọi trường cấu hình ánh xạ sang `TRACEPILOT_<MỤC>_<TRƯỜNG>`. Migration chạy chủ động (`tracepilot migrate up`), server không tự sửa schema khi khởi động.

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

**Postman.** Import `docs/tracepilot.postman_collection.json` (biến `baseUrl`, `otlpUrl`, `adminKey` nằm trong file) và chạy thư mục 1 đến 5, hoặc `npx newman run docs/tracepilot.postman_collection.json`. Nó tạo key, gửi trace/log/metric, xác nhận không credential nào được lưu, và kiểm tra 401/403/413/429 cùng việc key bị revoke dừng ngay. Request "Bad key, repeat 65x" chặn cả địa chỉ gọi trong một phút, nên chạy riêng.

**Browser.** Với DB đã migrate và seed cùng server đang chạy: `npm --prefix frontend exec playwright -- install chromium`, đặt `TRACEPILOT_E2E_BASE_URL` và `TRACEPILOT_E2E_API_KEY`, rồi `npm --prefix frontend run test:e2e`.

---

## 🤝 Đóng góp

Thảo luận thay đổi lớn trong issue trước; sửa nhỏ có thể gửi thẳng pull request. Giữ phạm vi hẹp, thêm test nhỏ nhất bảo vệ hành vi mới, không thêm dependency hạ tầng khi chưa có số đo. Không commit credential hay telemetry thật; chỉ dùng dữ liệu tổng hợp. Mỗi file nguồn giữ dưới 300 dòng (CI kiểm tra).

## 🔒 Bảo mật

Không mở issue công khai cho nghi vấn lỗ hổng liên quan đến lộ credential, truy cập chéo project, SSRF của replay, lạm dụng webhook hay đọc file tùy ý. Dùng tính năng báo cáo lỗ hổng riêng tư của repository, kèm phiên bản hoặc commit, bước tái hiện bằng dữ liệu tổng hợp và tác động quan sát được. Chủ repo cần công bố một đầu mối bảo mật được theo dõi trước bản phát hành đầu tiên. Hiện chỉ bản gắn tag mới nhất và `main` được vá.

Triển khai production phải: bật xác thực, TLS cho management/OTLP và PostgreSQL `verify-full`, thay khóa bootstrap mẫu; dùng API key tối thiểu quyền (`ingest` cho ứng dụng, `read` cho tra cứu, `admin` chỉ cho quản lý key và webhook) và revoke khóa bootstrap khi đã có admin key; để replay tắt trừ khi đã duyệt allowlist; chỉ dùng webhook HTTPS với encryption key trong secret manager; không ghi API key, secret webhook hay telemetry thô vào log. TracePilot chưa được duyệt cho telemetry production cho đến khi chủ repo chấp nhận các tiêu chí trong [OPERATIONS.md](docs/OPERATIONS.md).

---

## 📄 Bản quyền

Dự án được phân phối dưới giấy phép [MIT](LICENSE).
