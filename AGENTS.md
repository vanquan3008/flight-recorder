# Cách làm việc trong repo

- Trao đổi và giải thích với chủ repo bằng tiếng Việt.
- Trước một phạm vi phát triển mới, đưa 2–3 phương án thực tế, nêu đánh đổi và
  đề xuất một phương án. Tiếp tục các việc đã được chọn mà không hỏi lại.
- Hướng đã chọn: một ứng dụng Go + React, phân module theo trách nhiệm;
  PostgreSQL riêng. Chỉ tách tiến trình khi có yêu cầu hoặc số đo chứng minh.
- Giữ các thay đổi chưa commit của người dùng. Không tự reset, commit, push,
  deploy hoặc chạy migration trên database thật.

# Ranh giới source

- `backend/cmd/tracepilot`: cấu hình, nối dependency, vòng đời tiến trình.
- `backend/internal/application`: use case và background workflow qua interface nhỏ.
- `backend/internal/domain`: model và quy tắc thuần, không phụ thuộc adapter/runtime.
- `backend/internal/ingest`: giao thức OTLP, normalization và queue có giới hạn.
- `backend/internal/transport/httpapi`: REST, auth boundary và response contract.
- `backend/internal/storage`: SQL, transaction, migration và partition.
- `frontend/src/features`: giao diện theo chức năng; phần dùng chung ở `shared`.
- Cấu hình backend duy nhất: `backend/.env`, mẫu `backend/.env.example`. Console có file riêng
  `frontend/.env.example` (biến build `VITE_*` và địa chỉ backend của nginx). PostgreSQL
  và ClickHouse là dịch vụ ngoài; Compose không dựng database cho production. Chạy backend
  bằng `go run` hoặc image build từ `backend/Dockerfile`, console bằng `npm run dev` hoặc
  image từ `frontend/Dockerfile`. Production dùng `deploy/compose.yaml` (migrate, backend,
  console; đọc thẳng `backend/.env` và `frontend/.env`). `docker-compose/` chỉ dựng
  ClickHouse cho máy dev. Không thêm env riêng cho dev/prod.

# Kiểm tra

- Thay đổi logic phải có test hồi quy nhỏ; chạy test liên quan, Go vet/build.
- Thay đổi frontend: typecheck, tests và build.
- SQL/migration: chạy integration trên PostgreSQL test có thể bỏ đi.
- Không gọi kết quả unit test là chứng nhận production. Ghi rõ kiểm tra bỏ qua.
- Bộ docs hiện hành: `README.md` (chạy, phát triển, đóng góp, bảo mật),
  `docs/FLOW.md` (mô tả luồng) và `docs/diagrams.html` (sơ đồ hoạt động, Mermaid).
  Đổi luồng thì cập nhật cả hai file này; tránh thêm báo cáo chồng chéo. `docs/tracepilot.postman_collection.json`
  là bộ kiểm tra API; cập nhật cùng khi đổi route hoặc hành vi ingest.
