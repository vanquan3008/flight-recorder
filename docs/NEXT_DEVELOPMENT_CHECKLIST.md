# Checklist phát triển sau Phase 1

Tài liệu này là backlog thực thi ngắn gọn. Quy ước trong tài liệu này:

- **P0**: go-live safety và drill production.
- **Phase 1**: release blockers trong `DEVELOPMENT_CHECKLIST.md`.
- Các phần dưới đây là công việc tiếp theo, không đánh đồng với Phase 1 đã hoàn tất
  trong source.

## Phase 1 — Hoàn tất trong source

- [x] Smoke dùng Compose project/volume riêng và cleanup có guard.
- [x] Retention xử lý nhiều batch, có deadline và metric backlog.
- [x] Management query cách ly theo project + environment.
- [x] Replay luôn có trạng thái audit cuối và chặn SSRF/private target.
- [x] File agent bỏ qua oversized/corrupt input mà vẫn tiến checkpoint.

Xác nhận còn thiếu cho Phase 1 là chạy `make smoke` trên Linux/CI. Máy hiện tại
không có Bash/WSL, nên không thể thay thế check đó bằng local PowerShell.

## Cổng go-live còn phải xác nhận ngoài repository

- [ ] Render manifest deployment thật: PostgreSQL chỉ có private network, không publish `5432`.
- [ ] Mount certificate thật; host/IP của `TRACEPILOT_HEALTHCHECK_URL` nằm trong certificate SAN.
- [ ] Nếu dùng CA nội bộ, mount read-only CA và đặt `TRACEPILOT_HEALTHCHECK_CA_FILE`.
- [ ] Commit đúng SHA, push PR, và xác nhận `lint`, `test`, `docker`, `smoke`, `ci-success` đều xanh.
- [ ] Chạy lại backup/restore với snapshot production đã mask và có phê duyệt dữ liệu.

## Phase 2 — API contract và access lifecycle

- [ ] Viết OpenAPI 3.1 cho toàn bộ `/api/v1`, gồm auth, scope, pagination và error schema.
- [ ] Thêm contract tests cho route quan trọng: traces, logs, incidents, replays và API keys.
- [ ] Chuẩn hóa `400` cho toàn bộ ID/enum/query invalid; `404` cho resource hợp lệ nhưng không tồn tại.
- [ ] Hoàn tất API-key lifecycle: create, rotate, revoke, last-admin protection và audit-event read API.
- [ ] Chốt contract incident: fingerprint, dedup window, status transition và reopen rule.

**Done khi:** một OpenAPI document là nguồn contract duy nhất và các endpoint chính có test request/response.

## Phase 3 — Data correctness và hiệu năng

- [ ] Đưa migration `up → down 1 → up` vào CI database sạch.
- [ ] Test duplicate OTLP delivery không tạo duplicate spans/logs/incidents.
- [ ] Test queue saturation, admission timeout và recovery khi PostgreSQL gián đoạn.
- [ ] Benchmark trace/log/incident query với fixture được mask có kích thước đại diện.
- [ ] Đặt SLO ingest throughput, query latency và retention backlog trước khi cân nhắc partitioning/scale-out.

**Done khi:** có số đo baseline, giới hạn vận hành và test regression cho data loss/duplicate.

## Phase 4 — Operations và security

- [ ] Viết runbook deploy, upgrade, migration rollback, backup/restore và incident response.
- [ ] Công bố security contact, quy trình API-key rotation và threat model ngắn cho auth/SSRF/data isolation.
- [ ] Chốt cách bảo vệ `/metrics` (mTLS, private network hoặc read API key) theo deployment thật.
- [ ] Tạo dashboard/alert tối thiểu: database unavailable, queue saturation, retention backlog, agent spool gần đầy.
- [ ] Đặt resource limits, secret-manager integration và retention policy cho production.

**Done khi:** người on-call có thể phục hồi dịch vụ từ runbook mà không cần đọc source.

## Phase 5 — Product scope sau backend v1

- [ ] Trace/log/incident UI, trace waterfall và log-to-trace navigation.
- [ ] Saved search, dashboard, alert rule và email/Slack/webhook delivery.
- [ ] OTLP metrics ingestion/query chỉ sau khi capacity và storage model được chốt.
- [ ] Load test và soak test 24 giờ; restart app/PostgreSQL trong lúc ingest.

**Không làm sớm:** partitioning, distributed storage và dashboard framework. Chỉ thêm khi benchmark/SLO chứng minh PostgreSQL đơn lẻ không còn đáp ứng.
