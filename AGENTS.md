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
- `web/src/features`: giao diện theo chức năng; phần dùng chung ở `shared`.
- Cấu hình duy nhất: `.env` ở root, mẫu `.env.example`, triển khai `compose.yaml`.
  Không thêm env/Compose riêng cho dev/prod; PostgreSQL local dùng profile `local`.

# Kiểm tra

- Thay đổi logic phải có test hồi quy nhỏ; chạy test liên quan, Go vet/build.
- Thay đổi frontend: typecheck, tests và build lại embedded assets.
- SQL/migration: chạy integration trên PostgreSQL test có thể bỏ đi.
- Không gọi kết quả unit test là chứng nhận production. Ghi rõ kiểm tra bỏ qua.
- Bộ docs hiện hành: `ARCHITECTURE`, `DEVELOPMENT`, `OPERATIONS`, `REVIEW`,
  `ROADMAP` trong `docs/`. Cập nhật đúng nơi, tránh thêm báo cáo chồng chéo.
