# Đánh giá source và kế hoạch hoàn thiện TracePilot

## 1. Kết luận

Source hiện ở mức **backend beta/RC nội bộ**, chưa nên gắn nhãn production-ready.

Nền tảng kỹ thuật hiện có:

- Go backend và PostgreSQL.
- Nhận OTLP traces/logs qua gRPC và HTTP.
- Redaction dữ liệu nhạy cảm.
- Bounded queue, batching và retry khi lưu dữ liệu.
- Management API `/api/v1`.
- API key theo scope.
- Incident và allowed request execution có kiểm soát.
- Retention, Prometheus metrics và file log agent.
- Docker image, Compose, migrations, CI và release workflow.

Smoke test thực tế đã qua luồng migration → readiness → OTLP log ingest → log query → incident creation.

Phạm vi hiện tại là **observability backend cho traces và logs**, chưa phải hệ thống quan sát hoàn chỉnh cho người dùng cuối vì chưa có UI, alert delivery, metrics ingestion, dashboard và quy trình vận hành đầy đủ.

## 2. Phạm vi hệ thống hiện tại

```text
Application / File Agent
        |
        v
OTLP gRPC/HTTP Receiver
        |
        v
Normalize + Redact + Validate
        |
        v
Bounded Queue + Batch Workers
        |
        v
PostgreSQL
        |
        +--> Trace/Log/Service API
        +--> Incident API
        +--> API-key management
        +--> Allowed request execution audit
        +--> Retention
        +--> Prometheus metrics
```

Ngoài phạm vi hiện tại:

- Giao diện web hoàn chỉnh.
- OTLP metrics ingestion/query.
- Alert rule và notification delivery.
- Saved search và dashboard.
- Distributed storage hoặc horizontal scaling.
- Quy trình backup/restore và production operations hoàn chỉnh.

## 3. Các vấn đề quan trọng

### 3.1. Working tree đang tái cấu trúc nhưng chưa được stage

Toàn bộ source Go cũ ở root đang bị xóa, trong khi `backend/`, migrations mới và scripts đang là untracked. Cần tạo commit rename/migration nguyên tử trước khi tiếp tục phát triển để tránh thiếu source khi merge hoặc release.

### 3.2. Smoke script có nguy cơ xóa volume dev

Smoke script dùng Compose project tạm nhưng volume PostgreSQL lại có tên cố định. Cleanup bằng `docker compose down --volumes` có thể xóa volume dev hiện hữu.

Đây là release blocker và phải được sửa trước khi cho developer chạy `make smoke` trên máy cá nhân.

### 3.3. Retention không theo kịp tải thấp

Mỗi chu kỳ retention chỉ xóa đúng một batch. Với mặc định 10.000 record mỗi giờ, tốc độ cleanup tối đa chỉ khoảng 2,8 record/giây. Nếu lượng dữ liệu hết hạn cao hơn tốc độ này, backlog sẽ tăng liên tục.

### 3.4. Ranh giới environment chưa được bảo toàn

Dữ liệu được ghi với `environment_id`, nhưng phần lớn management query chỉ lọc `project_id`. Khi đổi environment hoặc chạy nhiều environment chung database, traces, logs và services có thể bị trộn.

### 3.5. Replay audit có thể kẹt ở trạng thái `started`

Việc hoàn tất replay execution dùng lại HTTP request context. Nếu client ngắt kết nối hoặc request timeout, thao tác cập nhật audit cũng có thể bị hủy và để bản ghi ở trạng thái `started` vĩnh viễn.

### 3.6. File agent có thể kẹt vĩnh viễn

Một dòng log vượt giới hạn trả lỗi nhưng không tiến checkpoint. Agent sẽ đọc lại và lỗi trên cùng dòng ở mọi tick.

API key được truyền qua command-line flag cũng có thể xuất hiện trong process list.

### 3.7. API chưa chuẩn hóa validation

Một số path ID được cast trực tiếp thành UUID trong SQL. ID sai định dạng có thể gây lỗi PostgreSQL và trả HTTP `500` thay vì `400`.

### 3.8. Release và operations còn thiếu

- Dependabot vẫn trỏ Go module ở `/`, trong khi `go.mod` đã chuyển vào `/backend`.
- Operations runbook cũ đang bị xóa.
- Chưa có security contact được giám sát.
- Chưa có OpenAPI contract.
- Chưa có backup/restore procedure.
- Chưa có upgrade và rollback guide.
- Trước Phase 0/1, một số bảng/cột như `payload_artifacts`, `last_used_at` và `result_payload_ref` chưa có luồng sử dụng hoàn chỉnh; các thành phần này đã được gỡ bằng migration có guard.

## 4. Kết quả kiểm tra

- [x] `go test ./...` chạy qua.
- [x] `go vet ./...` chạy qua.
- [x] `gofmt` sạch.
- [x] `go mod verify` chạy qua.
- [x] Docker image build thành công.
- [x] Migration trên PostgreSQL disposable chạy thành công.
- [x] Readiness check chạy thành công.
- [x] OTLP log ingestion chạy thành công.
- [x] Log query chạy thành công.
- [x] Incident creation chạy thành công.
- [ ] Race test cục bộ chưa chạy được vì Windows thiếu GCC/CGO.
- [ ] Fresh coverage run bị Application Control chặn một số executable trong thư mục Temp.
- [ ] `staticcheck` và `govulncheck` chưa có trên máy local.
- [ ] Trạng thái CI thực tế trên GitHub chưa được xác minh trong lần rà soát này.

## 4.1. Đã xử lý trong Phase 0/1

- [x] Smoke Compose dùng project/volume riêng, có guard cleanup và timeout tổng.
- [x] Retention xử lý nhiều batch trong một cycle có deadline.
- [x] Toàn bộ management query đã giữ `project_id` và `environment_id` trong scope.
- [x] Replay audit không phụ thuộc vòng đời HTTP client và có cleanup execution bị treo.
- [x] Replay integration test xác nhận client disconnect vẫn finalize audit execution.
- [x] Agent bỏ qua oversized line có kiểm soát, tiến checkpoint và phát metric.
- [x] Path ID phổ biến được validate trước khi chạm PostgreSQL.
- [x] Xóa schema scaffold không được sử dụng: `payload_artifacts`, hai payload reference và `api_keys.last_used_at`.

Các migration cleanup đều có guard: nếu database cũ có dữ liệu thật trong cột bị xóa, migration sẽ dừng thay vì âm thầm làm mất dữ liệu. Legacy spool format `[]string` vẫn được giữ tạm thời để không làm mất telemetry đang nằm trong disk buffer cũ.

## 5. Kế hoạch checklist

### P0 — Chốt backend v1 an toàn

#### Repository và release gate

- [ ] Stage và commit việc chuyển root module vào `backend/` như một thay đổi nguyên tử.
- [ ] Kiểm tra clean clone có đầy đủ source, migrations và scripts.
- [x] Sửa Dependabot directory từ `/` thành `/backend`.
- [x] Sửa smoke script để sinh volume name riêng theo project hoặc run ID.
- [x] Thêm guard không cho smoke cleanup volume dev.
- [ ] Xác nhận `make ci`, `make smoke` và release workflow hoạt động từ clean clone.

#### Data correctness

- [x] Sửa retention để chạy nhiều batch có giới hạn thời gian cho tới khi hết dữ liệu quá hạn.
- [x] Thêm test retention với backlog lớn hơn một batch.
- [x] Kiểm tra cascade delete cho events, logs, spans, incidents và executions.
- [x] Chốt v1 là một project/environment cho mỗi deployment.
- [x] Truyền `environment_id` vào toàn bộ trace/log/service/incident query.
- [x] Thêm integration test chống trộn dữ liệu giữa project và environment.

#### Replay và audit

- [x] Dùng context độc lập có timeout để luôn ghi kết quả replay audit.
- [x] Có cơ chế đánh dấu execution `started` quá hạn thành `failed`.
- [x] Kiểm tra SSRF policy với DNS rebinding, redirect, private IP và host allowlist.
- [x] Quyết định allowlist cho phép mọi port của host.

#### API và agent

- [x] Validate UUID, trace ID, span ID và enum trước khi gọi database.
- [x] Chuẩn hóa lỗi input thành HTTP `400`.
- [x] Cho agent bỏ qua hoặc quarantine dòng log quá lớn và tiến checkpoint có kiểm soát.
- [x] Thêm test log rotation, copytruncate và oversized line.
- [x] Hỗ trợ API key agent qua environment hoặc file secret.
- [x] Giữ CLI API-key flag chỉ cho development và cảnh báo khi sử dụng.

#### Security

- [ ] Công bố security contact được giám sát.
- [ ] Khôi phục hoặc viết lại operations/security runbook.
- [x] Thêm production guard khi bind public mà không có authentication hoặc TLS.
- [ ] Xác định rõ `/metrics` được public, bảo vệ bằng reverse proxy hay authentication.
- [ ] Tài liệu hóa secret rotation cho bootstrap key và API keys.

### P1 — Hoàn thiện contract và chất lượng

#### API contract

- [ ] Viết OpenAPI cho toàn bộ `/api/v1`.
- [ ] Mô tả error schema, pagination cursor và API-key scope của từng endpoint.
- [ ] Thêm contract test cho status code và response body.
- [ ] Chuẩn hóa tên query parameter và JSON field.

#### Test và reliability

- [ ] Bổ sung integration test cho toàn bộ endpoint database-backed.
- [ ] Chạy migration `up → down 1 → up` trong database sạch ở CI.
- [ ] Test PostgreSQL outage và recovery.
- [ ] Test queue saturation, admission timeout và retry.
- [ ] Test graceful shutdown không mất dữ liệu đã acknowledge.
- [ ] Test corrupt checkpoint, corrupt spool và disk-full.
- [ ] Đặt coverage gate cho các package quan trọng thay vì ép coverage tổng.

#### Schema cleanup

- [x] Xóa `payload_artifacts` và các reference chưa được sử dụng.
- [x] Xóa `result_payload_ref` chưa được sử dụng.
- [x] Xóa `api_keys.last_used_at` vì không có producer/consumer.
- [x] Không phát sinh write database trên mọi request cho `last_used_at`.
- [ ] Thêm API đọc `api_key_events` hoặc bỏ bảng nếu audit này chưa thuộc v1.

#### CI và supply chain

- [ ] Pin toàn bộ GitHub Actions bằng commit SHA.
- [ ] Bảo đảm `staticcheck`, `govulncheck` và race test là release gate bắt buộc.
- [ ] Không cho coverage upload failure che lỗi release quan trọng.
- [ ] Kiểm tra release binary và Docker image bằng smoke test từ artifact đã build.

### P2 — Production operations

#### Backup và deployment

- [ ] Viết tài liệu backup PostgreSQL.
- [ ] Viết tài liệu restore và diễn tập restore thực tế.
- [ ] Viết upgrade/rollback runbook cho binary, image và migration.
- [ ] Tách cấu hình Compose development khỏi deployment production.
- [ ] Không expose PostgreSQL port trong cấu hình production mặc định.

#### Observability của chính TracePilot

- [ ] Dashboard Prometheus cho ingest rate và rejection rate.
- [ ] Dashboard queue depth, inflight bytes và batch latency.
- [ ] Dashboard database readiness và storage errors.
- [ ] Bổ sung retention backlog/oldest-event metric.
- [ ] Alert cho queue saturation, retention backlog và DB unavailable.
- [ ] Alert cho replay failure và agent spool gần đầy.

#### Performance và resilience

- [ ] Benchmark ingest theo batch size, workers và DB connections.
- [ ] Benchmark trace query và log search với dataset đại diện.
- [ ] Benchmark retention trong khi hệ thống vẫn ingest.
- [ ] Soak test tối thiểu 24 giờ.
- [ ] Restart backend và PostgreSQL trong soak test.
- [ ] Kiểm tra trùng và mất dữ liệu sau test.
- [ ] Chỉ thêm partitioning hoặc storage khác khi benchmark chứng minh PostgreSQL không đáp ứng.

#### Release artifact

- [ ] Xuất SBOM.
- [ ] Ký Docker image và release artifact.
- [ ] Công bố checksum và hướng dẫn xác minh.
- [ ] Kiểm tra image chạy non-root và read-only filesystem.

### P3 — Hoàn thiện sản phẩm sau backend v1

- [ ] UI tìm kiếm traces và logs.
- [ ] Trace waterfall và span detail.
- [ ] Incident workflow và lịch sử trạng thái.
- [ ] Replay execution viewer.
- [ ] Saved searches và dashboard.
- [ ] Alert rule và notification delivery.
- [ ] SDK/Collector onboarding examples.
- [ ] Sample application và synthetic telemetry fixtures.
- [ ] Chỉ bổ sung OTLP metrics nếu sản phẩm chuyển từ trace/log backend thành observability platform đầy đủ.

## 6. Thứ tự triển khai đề xuất

### Giai đoạn 1 — Release blockers

1. Hoàn tất repository move.
2. Sửa smoke volume safety.
3. Sửa retention backlog.
4. Bảo toàn environment boundary.
5. Sửa replay audit context.
6. Sửa agent oversized-line handling.

### Giai đoạn 2 — Contract và test

1. Chuẩn hóa input validation.
2. Viết OpenAPI.
3. Bổ sung integration và isolation tests.
4. Hoàn thiện CI gates.
5. Dọn schema chưa sử dụng.

### Giai đoạn 3 — Production readiness

1. Backup/restore drill.
2. Upgrade/rollback runbook.
3. Monitoring và alerting nội bộ.
4. Load, outage và soak test.
5. Supply-chain hardening.

### Giai đoạn 4 — Product experience

1. Frontend trace/log explorer.
2. Incident workflow.
3. Alerting và notifications.
4. Onboarding và examples.

## 7. Definition of Done cho backend v1

- [ ] Clean clone chạy được `make ci`.
- [ ] Clean clone chạy được smoke mà không đụng dữ liệu dev.
- [ ] CI race, staticcheck, vulnerability scan và integration test đều xanh.
- [ ] Không trộn dữ liệu giữa project hoặc environment.
- [ ] Không mất dữ liệu đã trả success khi shutdown bình thường.
- [ ] Retention bắt kịp tốc độ ingest đã công bố.
- [ ] Agent không bị kẹt bởi malformed hoặc oversized log line.
- [ ] Replay luôn có trạng thái kết thúc và audit record đầy đủ.
- [ ] Backup đã được restore thử thành công.
- [ ] OpenAPI, operations và security docs được phát hành cùng version.
- [ ] Docker image chạy non-root, read-only và không yêu cầu secret mặc định trong image.
- [ ] Release artifact có checksum và được kiểm tra bằng smoke test.

## 8. Những phần chưa nên xây ngay

Để giữ hệ thống đơn giản và tránh over-engineering, chưa nên thêm các thành phần sau nếu chưa có benchmark hoặc yêu cầu sản phẩm rõ ràng:

- Kafka hoặc message broker riêng.
- ClickHouse hoặc storage thứ hai.
- Kubernetes operator.
- Microservice hóa backend hiện tại.
- Custom cache layer.
- Database partitioning phức tạp.
- OTLP metrics chỉ để đủ bộ tính năng.

PostgreSQL, bounded queue và một backend binary vẫn là kiến trúc phù hợp cho v1. Chỉ mở rộng khi số liệu tải thực tế chứng minh giới hạn.
