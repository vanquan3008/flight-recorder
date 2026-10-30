# TracePilot

TracePilot là nền tảng quan sát và giám sát hiệu năng ứng dụng (APM & Observability) tự lưu trữ (self-hosted) dành cho các nhóm kỹ thuật. TracePilot tiếp nhận dữ liệu chuẩn OpenTelemetry (OTLP) gồm traces, application logs và metrics; tự động liên kết tương quan giữa sự cố (incidents) và sơ đồ quan hệ phụ thuộc dịch vụ (service topology), đồng thời xếp hạng giả thuyết nguyên nhân gốc rễ (Root Cause Analysis - RCA) với bằng chứng trực quan.

**Trạng thái hiện tại: Beta.** Quy trình nghiệm thu môi trường vận hành thực tế được quy định tại [OPERATIONS.md](docs/OPERATIONS.md). Dữ liệu traces, logs và OTLP metrics cam kết chỉ phản hồi ACK khi transaction PostgreSQL đã commit thành công.

---

## 🌟 Điểm nổi bật

- **Tương thích toàn diện OpenTelemetry (OTLP):** Hỗ trợ đầy đủ giao thức gRPC (`:4317`) và HTTP/JSON (`:4318`) cho `/v1/traces`, `/v1/logs`, `/v1/metrics`.
- **Kiến trúc tinh gọn (Single-binary Monolith):** Một tiến trình Go duy nhất nhúng sẵn toàn bộ Web Console (React), lưu trữ trực tiếp vào PostgreSQL (riêng biệt). Không cần thêm middleware phức tạp.
- **Giao diện Web Console hiện đại:** Xây dựng với React và Tailwind CSS, hỗ trợ giao diện Sáng/Tối (Dark/Light mode), thanh điều hướng Capsule Dock nổi, bố cục Bento Card trực quan.
- **Trace Explorer & Waterfall:** Xem chi tiết luồng thực thi yêu cầu, phân tích cây span, phát hiện điểm nghẽn độ trễ và lỗi phân tán.
- **Log Correlation:** Tìm kiếm log độc lập theo khung thời gian hoặc truy vết chéo ngay lập tức với trace/span tương ứng.
- **Bản đồ cấu trúc dịch vụ (Topology Graph):** Trực quan hóa tương tác giữa các service nội bộ và hạ tầng bên thứ ba.
- **Phát hiện sự cố & Đề xuất RCA:** Tự động phát hiện lỗi bất thường qua baseline tín hiệu, gom nhóm sự cố và đưa ra giả thuyết nguyên nhân gốc rễ kèm bằng chứng định lượng.
- **Bảo vệ hệ thống & An toàn tài nguyên:** Pipeline tiếp nhận có giới hạn bộ nhớ (byte budget) và hàng đợi có chặn, tự động loại trừ dữ liệu nhạy cảm (redaction), hỗ trợ phân vùng (partition) bảng theo ngày.

---

## 🚀 Khởi chạy nhanh trên Local

Yêu cầu môi trường: **Docker Compose v2**. Nếu phát triển frontend trực tiếp, dùng Node theo `frontend/.nvmrc`; backend Go theo `backend/go.mod`.

### 1. Khởi chạy toàn bộ với Docker Compose

```powershell
# 1. Cài đặt dependency và build giao diện web nhúng
npm --prefix frontend ci
npm --prefix frontend run build

# 2. Tạo file cấu hình từ mẫu (nếu chưa có)
if (-not (Test-Path .env)) { Copy-Item .env.example .env }

# 3. Khởi chạy PostgreSQL và chạy migrate cấu trúc dữ liệu
docker compose up -d --wait postgres
docker compose run --build --rm migrate

# 4. Khởi chạy máy chủ TracePilot
docker compose up --build -d tracepilot

# 5. Khởi tạo dữ liệu mẫu (mockdata) để trải nghiệm
docker compose run --rm tracepilot mockdata
```

### 2. Truy cập Web Console

Mở trình duyệt tại: [http://localhost:8080](http://localhost:8080)

Nhập mã khóa khởi tạo dành riêng cho môi trường phát triển (bootstrap key):
```text
tracepilot-development-bootstrap-key-000000
```

> [!TIP]
> Sau khi vào console, hãy vào mục **Settings** để tạo Ingestion API Key riêng cho các ứng dụng cần gửi dữ liệu, và Read API Key cho việc truy vấn hàng ngày.

---

## 🛠️ Chế độ phát triển Native (Go + Vite)

Dành cho nhà phát triển muốn debug backend hoặc làm giao diện frontend với tính năng hot-reload:

```powershell
# Đảm bảo file .env ở thư mục gốc có TRACEPILOT_DATABASE_URL trỏ về localhost
# Chạy migration và nạp mockdata qua Go CLI
go -C backend run ./cmd/tracepilot migrate up
go -C backend run ./cmd/tracepilot mockdata
go -C backend run ./cmd/tracepilot serve
```

Trên một cửa sổ terminal khác:
```powershell
npm --prefix frontend ci
npm --prefix frontend run dev
```
Vite Dev Server sẽ chạy tại [http://localhost:5173](http://localhost:5173) và tự động proxy các yêu cầu `/api/v1` và `/health` về backend Go cổng `:8080`.

---

## 📡 Cổng giao tiếp & Giao thức

| Dịch vụ | Cổng | Giao thức / Đường dẫn | Mô tả |
| :--- | :--- | :--- | :--- |
| **OTLP gRPC** | `:4317` | gRPC | Tiếp nhận Traces, Logs, Metrics từ OpenTelemetry SDK/Collector |
| **OTLP HTTP** | `:4318` | HTTP/Protobuf hoặc JSON | Endpoints: `/v1/traces`, `/v1/logs`, `/v1/metrics` |
| **Web Console** | `:8080` | HTTP | Giao diện quản trị, tra cứu và điều hành |
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
    Console["React Web Console\n(Tailwind CSS)"] <-->|"REST API (:8080)"| HTTPAPI["HTTP API Layer"]
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
│       └── webui/               # Adapter nhúng tài nguyên frontend vào binary Go
├── frontend/
│   └── src/
│       ├── app/                 # Shell ứng dụng, router, xác thực phiên
│       ├── features/            # Các trang chức năng (overview, traces, logs, topology, settings...)
│       ├── shared/              # Thành phần dùng chung (navigation, paged table, graph layout...)
│       └── styles/              # Base, components, Tailwind CSS và responsive styles
├── docs/                        # Tài liệu kỹ thuật chuyên sâu (5 tài liệu tiêu chuẩn)
└── compose.yaml                 # Cấu hình container tiêu chuẩn cho toàn dự án
```

---

## ⚙️ Cấu hình môi trường

Tất cả cấu hình hệ thống được định nghĩa thống nhất tại một file độc lập duy nhất: `.env` ở thư mục gốc (tham khảo mẫu `.env.example`).

Các biến cốt lõi:
- `TRACEPILOT_DATABASE_URL`: Chuỗi kết nối PostgreSQL (ví dụ `postgres://tracepilot:secret@postgres:5432/tracepilot?sslmode=disable`).
- `TRACEPILOT_ADMIN_API_KEY`: Khóa quản trị hệ thống dùng trong trường hợp bootstrap.
- `COMPOSE_PROFILES`: Đặt `local` để bật PostgreSQL nội bộ trong Docker Compose; bỏ trống khi triển khai trên production với cơ sở dữ liệu bên ngoài.
- `TRACEPILOT_RETENTION_TELEMETRY_DAYS`: Số ngày lưu trữ traces/logs/metrics (mặc định: `7` ngày).
- `TRACEPILOT_RETENTION_INCIDENTS_DAYS`: Số ngày lưu trữ sự cố (mặc định: `90` ngày).

---

## 📚 Danh mục tài liệu kỹ thuật

Toàn bộ tài liệu chi tiết được duy trì tập trung trong thư mục `docs/`:

| Tài liệu | Nội dung chính |
| :--- | :--- |
| [ARCHITECTURE.md](docs/ARCHITECTURE.md) | Kiến trúc chi tiết, trách nhiệm từng package, luồng dữ liệu và thiết kế phân lớp |
| [DEVELOPMENT.md](docs/DEVELOPMENT.md) | Hướng dẫn phát triển, thiết lập môi trường Native/Compose, kiểm thử tự động |
| [OPERATIONS.md](docs/OPERATIONS.md) | Vận hành sản xuất, sao lưu/phục hồi, bảo mật TLS, kiểm thử tải và tiêu chí nghiệm thu |
| [REVIEW.md](docs/REVIEW.md) | Báo cáo kiểm định chất lượng mã nguồn, kết quả benchmark và các giới hạn đã biết |
| [ROADMAP.md](docs/ROADMAP.md) | Lộ trình phát triển, các giải pháp cân nhắc và tiêu chuẩn tiếp nhận tính năng |

---

## 🧪 Kiểm tra chất lượng & Tiêu chuẩn đóng góp

Trước khi tích hợp mã nguồn, toàn bộ các bài kiểm thử phải vượt qua:

```powershell
# 1. Kiểm tra Backend (Go)
go -C backend test ./...
go -C backend vet ./...
go -C backend build ./...

# 2. Kiểm tra Frontend (React + TypeScript)
npm --prefix frontend run check    # Kiểm tra kiểu TypeScript (tsc --noEmit)
npm --prefix frontend run test     # Chạy unit/integration test (vitest)
npm --prefix frontend run build    # Kiểm tra build và đồng bộ embedded assets
```

Mọi đóng góp vui lòng tham khảo [CONTRIBUTING.md](CONTRIBUTING.md) và báo cáo bảo mật qua [SECURITY.md](SECURITY.md).

---

## 📄 Bản quyền

Dự án được phân phối dưới giấy phép [MIT](LICENSE).
