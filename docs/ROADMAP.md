# TracePilot roadmap

Checklist này chỉ giữ việc chưa hoàn tất. Kết quả đã xong phải được bảo vệ bằng
test, migration hoặc workflow trong source thay vì tiếp tục nằm trong một danh
sách lịch sử dài.

Quy ước ưu tiên: **P0** chặn refactor/release, **P1** cần trước production,
**P2** hoàn thiện product, **P3** chỉ làm khi có nhu cầu đo được.

## P0 — Source dễ maintain

- [x] Gom tài liệu còn README, Architecture, Development, Operations và Roadmap.
- [x] Gom fixed-load và timed-soak vào một script; bỏ wrapper backup/restore.
- [x] Giữ file Go viết tay dưới khoảng 300 dòng. Chỉ tách tại ranh
      giới trách nhiệm; generated code, migration và fixture không bị ép theo
      giới hạn này.
- [x] Tách config validation theo HTTP/ingestion và security/runtime.
- [x] Tách Prometheus registry construction khỏi observer methods.
- [x] Tách durable pipeline admission/lifecycle khỏi worker/persistence.
- [x] Tách metric normalizer theo number và histogram/summary.
- [x] Tách security detection orchestration khỏi query/incident persistence.
- [x] Tách CSS thành layout, component, responsive và trace detail.
- [x] Chia integration test trên 300 dòng theo behavior, không theo số dòng.
- [ ] Chuyển SQL tĩnh có giá trị cao sang `db/queries`/SQLC; giữ SQL động an toàn
      bằng parameter và allowlist khi generated query làm code khó hiểu hơn.
- [x] Thêm CI check phát hiện file production viết tay vượt 300 dòng nhưng cho
      phép exception có giải thích.

Gate: `gofmt`, unit/integration test, architecture test, race, vet, staticcheck,
govulncheck và `git diff --check` đều xanh; API/schema/durability không đổi.

## P1 — Correctness và isolation trước production

- [ ] Test queue saturation và PostgreSQL outage không deadlock/leak goroutine.
- [ ] Test shutdown khi queue còn dữ liệu và DB chưa hồi phục.
- [ ] Test detection checkpoint rollback khi một insert thất bại.
- [ ] Hoàn thành matrix cross-project/cross-environment cho mọi query/mutation.
- [ ] Test agent crash/restart với spool đầy, corrupt, truncate và file replace.
- [ ] Test startup với schema dirty, cũ và mới hơn binary; chốt compatibility
      application/schema N và N-1.
- [ ] Fuzz parser CEF/LEEF/Syslog/JSON và OTLP attribute recursion/size limits.
- [ ] Test replay với redirect, DNS rebinding, mixed DNS answers và private IP.
- [ ] Audit log không lộ API key, DSN, webhook secret hoặc raw sensitive body.
- [ ] Công bố security contact được giám sát trước public release.

Gate: không có cross-tenant leak, false ACK hoặc critical/high security finding
chưa có quyết định xử lý.

## P1 — PostgreSQL và vận hành

- [ ] Đặt RPO/RTO và retention riêng cho raw event, metric, detection, incident
      và audit record.
- [ ] Benchmark threshold detection theo event/rule/window; tối ưu SQL chỉ sau
      khi query plan chứng minh bottleneck.
- [ ] Chạy `EXPLAIN ANALYZE` cho trace list/detail, log correlation, dependency
      aggregation, detection và retention trên dataset gần production.
- [ ] Theo dõi pool saturation, slow query, WAL, autovacuum và table/index size.
- [ ] Drill certificate rotation, deploy/restart, application rollback,
      migration rollback, backup và restore trên staging.
- [ ] Định nghĩa SLI/SLO cho ingest availability/commit latency, query latency,
      detection lag và durability.
- [ ] Tạo dashboard/alert cho DB, queue, Syslog source lag, agent spool,
      detection checkpoint, webhook failure, backup age, disk và TLS expiry.
- [ ] Chạy timed soak ở 2x tải dự kiến; ghi p50/p95/p99, error rate, CPU, RAM,
      WAL và queue depth.

Gate: release candidate chạy soak, backup/restore và rollback thành công trên
staging tương đương production.

## P2 — Trace và security workflow

- [ ] Trace Explorer giữ filter trong URL và có loading/empty/error/retry state.
- [ ] Waterfall xác định critical path đúng với nested, sibling và missing parent.
- [ ] Correlate trace/log với incident occurrence và raw security evidence.
- [ ] Service dependency aggregation không đếm duplicate span.
- [ ] Security filter theo source/destination IP, port, vendor, action, outcome,
      severity, kind và time range.
- [ ] Source inventory hiển thị last seen, accepted/rejected, parse error và lag.
- [ ] Detection detail hiển thị rule revision, matched field và evidence.
- [ ] Rule test endpoint chạy sample nhưng không tạo detection thật.
- [ ] Incident detail bổ sung replay history; mọi list có pagination/load-more.
- [ ] Hoàn thiện keyboard navigation, focus, contrast và responsive table.

Gate: operator đi từ trace lỗi đến span/log nguyên nhân, hoặc từ firewall raw
event đến detection/incident, trong tối đa ba thao tác.

## P2 — Release và supply chain

- [ ] Rename remote repository từ `flight-recorder` sang `tracepilot`.
- [ ] Chạy clean-clone CI trên pull request và branch/tag remote thật.
- [ ] Chốt semantic version, changelog và release-candidate flow.
- [ ] Scan image/license; deploy bằng immutable digest đã ký.
- [ ] Xác nhận production chỉ publish port cần thiết, container non-root,
      read-only, có CPU/RAM limit và log rotation.

Gate: checksum, SBOM, signature/provenance, smoke test và CI đều gắn với cùng
immutable release digest.

## P3 — Chỉ sau core go-live

- [ ] Saved searches phía server.
- [ ] Multi-project administration và fine-grained RBAC.
- [ ] Project quota cùng export/delete workflow.
- [ ] Vendor rule packs sau khi có sanitized fixtures.
- [ ] Nhiều replica/leader election khi single replica không đạt SLO.
- [ ] Kubernetes/Helm chỉ khi Compose không còn đáp ứng topology thực tế.
- [ ] Partitioning chỉ khi retention benchmark chứng minh batch delete không đủ.

## Definition of done cho production

- [ ] Contract ingest/API và project isolation được test end-to-end.
- [ ] Trace và security workflow đạt acceptance gate P2.
- [ ] Outage, shutdown drain, agent spool và detection retry không mất/nhân dữ liệu
      ngoài contract đã công bố.
- [ ] Staging đạt SLO và hoàn thành deploy, backup, restore, upgrade, rollback.
- [ ] Dashboard, alert và runbook bao phủ failure mode chính.
- [ ] Release artifact/image có checksum, SBOM, signature và provenance.
