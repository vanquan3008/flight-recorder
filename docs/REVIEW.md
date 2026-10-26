# Đánh giá source và kết quả kiểm tra

Ngày 13/10/2026, trên working tree hiện tại. Repo có nhiều thay đổi chưa commit
từ trước phiên này; báo cáo không quy toàn bộ git diff cho đợt làm việc này.

Repo này làm gì: nhận OTLP traces/logs/metrics, lưu PostgreSQL, truy vấn qua REST
và React console, tạo incidents/evidence/topology và xếp hạng giả thuyết RCA.
Giả định tải: một replica trên mạng riêng, PostgreSQL riêng, chưa có peak/SLO
production được chủ repo xác nhận.

## Lỗi đã sửa

| # | Trường hợp cụ thể và hậu quả trước đây | Thay đổi và kiểm chứng |
| --- | --- | --- |
| 1 | Metrics chỉ giới hạn số point. Nhiều request chứa labels/histogram lớn có thể giữ lượng byte vượt RAM dù chưa chạm point limit. | Thêm byte reservation cho queue và active write, giải phóng khi success/failure; histogram +Inf không đi qua JSON estimate. Unit regression pass. |
| 2 | Load tool gửi cùng payload cho mọi request. 1000 request x 20 log có thể chỉ tạo 20 unique logs; HTTP 200 partial rejection vẫn được tính thành công. | Tạo identity/timestamp theo request, đếm accepted/rejected, partial rejection là failure, kiểm tra worker/result count. Self-check pass; live 10 x 3 tạo đúng 30 unique logs. |
| 3 | CREATE partition lỗi permission/disk/connection bị rollback savepoint rồi trả nil. Startup có thể báo thành công dù maintenance không thực hiện được. | Chỉ bỏ qua PostgreSQL check violation 23514 của fallback dự kiến; lỗi khác trả về. Unit test và integration default-partition fallback pass. |
| 4 | Generated SQLC chưa phản ánh các bảng default partition trong baseline hiện tại; generator có thể làm CI diff thất bại. | Sinh lại bằng version v1.31.1 đã ghim trong Makefile; sinh lần hai không đổi hash. Integration compile/query pass. |
| 5 | Production env mẫu có biến rate limit nhưng Compose không chuyển chúng vào container; operator đổi giá trị vẫn chạy default. | Cấu hình đã gom vào root .env + compose.yaml; env_file chuyển toàn bộ field vào container, không còn danh sách chuyển tiếp thiếu biến. |
| 6 | Docs mô tả migration 014/015 và trace scope cũ trong khi source chỉ có baseline 1 và query đã scope environment. Workflow nằm rải rác ở internal root. | Thay 10 docs bằng 5 đầu mối hiện hành; gom workflow vào application, cập nhật callers/tests và tăng architecture guard. Không đổi REST/schema vì di chuyển package. |

## Cần giải quyết trước production

### 7. Secret trong free-text vẫn được lưu

- Là gì: normalizer redact theo tên attribute; LogBody, exception message hoặc
  URL có thể chứa nội dung tự do.
- Vấn đề: log body chứa password hoặc token vẫn có thể được lưu và đi vào
  incident/evidence dù key nhạy cảm trong attributes đã được che.
- Cách xử lý: chốt policy, lọc ở SDK/Collector trước; bổ sung server rule theo
  fixture cụ thể nếu cần. Test cả DB, evidence, webhook và internal logs.
- Nếu bỏ qua: dữ liệu nhạy cảm có thể bị lưu trái với policy của deployment.
  Chưa có policy/fixture production từ chủ repo để xác minh trọn vẹn.

### 8. Baseline không phải đường nâng cấp database cũ

- Là gì: migrations.CurrentVersion=1 và chỉ còn 000001_init từ trạng thái repo
  trước phiên này.
- Vấn đề: DB version 15 cũ không thể chuyển sang source hiện tại bằng migrate up;
  server từ chối version khác. Recreate DB sẽ làm mất dữ liệu nếu chưa backup.
- Cách xử lý: xác nhận có DB cũ không; giữ binary cũ hoặc viết data migration với
  backup/restore và đối chiếu row counts. Chỉ tạo DB mới nếu dữ liệu được phép bỏ.
- Nếu bỏ qua: rollout lên DB cũ thất bại hoặc operator có thể chọn cách xóa dữ
  liệu để chạy lại. Phiên này chỉ migrate cluster test mới, không dùng DB hiện có.

### 9. Chính sách lưu chưa giới hạn toàn bộ database (đã xử lý 24/10/2026)

Chủ repo chọn 7/90/180/30 ngày cho telemetry, incident, audit, delivery; đã triển
khai theo nhóm, cấu hình được (mục "Đợt quyết định production"). Còn lại: dung lượng
đĩa vẫn cần ước lượng theo volume thật; lịch sử đổi trạng thái incident sống
cùng incident (≥90 ngày), ngắn hơn audit key 180 ngày.

## Nên hoàn thiện

### 10. Đóng incident chưa có quy tắc reopen theo occurrence mới

- Là gì: incident writer tìm cùng fingerprint trong cửa sổ một giờ, không lọc
  status khi thêm occurrence.
- Vấn đề: lỗi mới trong cửa sổ có thể được thêm vào incident RESOLVED mà status
  vẫn RESOLVED; webhook incident.opened không được phát lại.
- Cách xử lý: chọn giữ dedup hiện tại hoặc reopen khi occurrence mới hơn thời
  điểm resolve, kèm actor/audit/outbox; delayed event và IGNORED có rule riêng.
- Nếu bỏ qua: operator cần biết resolved không đồng nghĩa không còn lỗi.
  Chưa đổi semantics khi chủ repo chưa chọn policy.

## Các giới hạn được giữ có chủ đích

- OTLP metrics đã ACK sau commit (24/10/2026). Chỉ metrics Prometheus của chính
  TracePilot nằm trong bộ nhớ và có thể mất khi tiến trình chết.
- Hai pipeline có budget riêng, không phải giới hạn tổng RSS. gRPC decode trước
  admission; cần load/RSS measurement trên payload và cardinality đại diện.
- Một server phục vụ environment cấu hình; query đã scope environment nhưng
  chưa có UX chuyển environment. Nhiều replica đã có test cho ingest, outbox,
  detection, partition và khởi động; rate limit/cổng truy vấn vẫn theo từng tiến trình.
- RCA là giả thuyết deterministic có evidence; chưa đánh giá độ đúng trên
  incident thật, không có LLM hoặc remediation.

## Đã chạy trong phiên này

| Kiểm tra | Kết quả |
| --- | --- |
| Go unit + architecture | go test ./... pass |
| PostgreSQL 17.9 native disposable | Migrate up/down/up pass, version 1 dirty=false; toàn bộ Go suite với INTEGRATION_TEST=1 pass |
| Go vet/build | Pass sau khi frontend assets hoàn chỉnh |
| Staticcheck 2026.1 | Pass |
| govulncheck | Không tìm thấy lỗ hổng đã biết trong kết quả scan |
| SQLC v1.31.1 | Sinh lại và kiểm tra hash ổn định pass |
| Frontend | Typecheck, 5 test files / 12 tests, production build pass |
| Browser smoke | 1/1 pass: connect → failed trace → correlated log |
| Load tool self-check | Unique records và partial rejection accounting pass |
| Live load kiểm tra tính đúng | 10 request, concurrency 2, 3 records/request; 30 accepted = 30 unique logs qua API, 0 failures |
| DB outage/recovery thật | Dừng cluster test: OTLP HTTP trả 503; restart và gửi lại cùng payload hai lần: đúng 1 unique log |
| Compose | config --quiet pass; local/external DB và one-shot migrate kiểm tra bằng model Compose |
| Format/source size | gofmt sạch; source thuộc size gate không vượt 300 dòng |
| Đóng môi trường test | Application ghi server stopped khi nhận Ctrl+C; PostgreSQL test đã dừng |

Live load nhỏ chỉ kiểm tra tính đúng. p95 khoảng 270 ms ở lần chạy đó bao gồm
đặc tính máy Windows test và client; không dùng làm throughput/SLA production.
PostgreSQL test được tạo mới ở loopback/cổng riêng; không dùng dữ liệu hiện có.
Thư mục dữ liệu PostgreSQL test cũ trong bin/ đã được xóa ngày 14/10/2026; không còn
application/DB test chạy.

Một lượt Go vet ban đầu trùng lúc Vite thay thư mục embedded dist nên báo thiếu
assets. Đã chạy lại sau frontend build và pass. Docker daemon không chạy;
PostgreSQL native và browser smoke được dùng để kiểm tra luồng tương ứng.

## Đợt React console (14/10/2026)

| Kiểm tra | Kết quả |
| --- | --- |
| Frontend typecheck, vitest | Pass: 12 file / 42 test (có test ranh giới import, layout topology, theme, logs, settings, quay lại giữ filter) |
| Production build | Pass; embedded assets build lại, Go vet/build, webui và architecture test pass; `go test ./...` toàn bộ pass |
| Browser (Chromium, API mock) | 9 màn × sáng/tối × 390/768/1440px: không tràn ngang, không lỗi console; theme giữ sau reload; đã xem ảnh các màn chính |
| Size gate web/src | Không file nào quá 300 dòng |
| Detection → application | `application/detection` điều phối ba detector và lưu incident theo service (thứ tự ổn định, lỗi detector không ghi gì); storage chỉ còn truy vấn và transaction. Unit test mới + toàn bộ Go suite với `TRACEPILOT_INTEGRATION_TEST=1` trên PostgreSQL 17 disposable (Docker, cổng loopback 55432) pass |
| Trace detail (flow + waterfall) | Test mô hình trace (cây, self time, edge giữa service, vòng cha, span mồ côi, trace rỗng) + 6 test hành vi; vitest 14 file / 58 test pass; Playwright 8/8 trên Go + PostgreSQL disposable; đã xem ảnh sáng/tối 1440 và 390px |
| Dọn repo/gate | Xóa thư mục rỗng `deploy/`, `docs/assets/`; sửa đường dẫn `backend/` trong AGENTS.md; gate assets trong CI/Makefile giờ bắt cả file dist mới chưa track (trước đây `git diff` bỏ sót); dependabot thêm npm và Docker; CONTRIBUTING/SECURITY/OPERATIONS có checklist mở traffic. Chưa chạy được `make` và CI trên máy này |
| E2E Go + PostgreSQL | Playwright 7/7 pass trên binary embed thật với DB đã migrate và seed: luồng chính, theme sau reload, Back giữ filter, log → trace → Back, topology, tạo/revoke API key, key chỉ-đọc vào Settings bị từ chối rõ ràng |

Browser check dùng fixture mock, chỉ xác minh trình bày/tương tác. Chưa chạy: zoom 200%, đọc bằng screen reader, đánh giá trên dữ liệu thật, trace/log
với volume lớn. Topology layout mới đo bằng unit test 400 node, chưa đo render trong browser.

## Đợt chất lượng tín hiệu ingest (24/10/2026)

| # | Trường hợp cụ thể và hậu quả trước đây | Thay đổi và kiểm chứng |
| --- | --- | --- |
| 11 | Fingerprint băm nguyên message; log ERROR lấy body làm message. "user 1 not found" và "user 2 not found" mở hai incident và hai webhook incident.opened. | `domain/incident/fingerprint.go` chuẩn hóa UUID, thời điểm, email, IP, hex, số trước khi băm; giữ khác biệt service/operation/type/status/message. Unit test nhóm và tách. Fingerprint mới không khớp incident đang mở tạo trước thay đổi: mỗi lỗi đang mở có thể mở thêm một incident mới một lần. |
| 12 | Mỗi event lỗi tốn khoảng 7 lượt gọi DB tuần tự trong transaction ingest; error storm làm chậm ACK, đầy queue và 503. Hai worker khóa fingerprint khác thứ tự có thể deadlock. | `storage/postgres/incidents` gom theo fingerprint: một advisory lock theo thứ tự sắp xếp, một truy vấn occurrence đã ghi, một lượt ghi incident và một `pgx.Batch` cho occurrence/evidence. Giữ cửa sổ dedup 1 giờ, idempotent khi retry, bỏ event lặp trong batch. Integration test trên PostgreSQL 17. |
| 13 | Một event PostgreSQL từ chối (vd. U+0000 trong jsonb) làm hỏng cả batch 256 event của nhiều request; retry 3 lần rồi 503 cho tất cả, exporter gửi lại và hỏng tiếp. | Normalizer từ chối từng record có NUL. Storage gắn `telemetry.ErrInvalidEvent` cho SQLSTATE lớp 22/23502/23514; pipeline không retry lỗi này, chia đôi batch để tách event hỏng; receiver trả partial success, metric rejected reason=invalid. `Submit` trả (persisted, rejected, error). Unit test pipeline/receiver/phân loại lỗi; integration xác nhận PostgreSQL thật từ chối NUL được phân loại đúng. |

| Kiểm tra | Kết quả |
| --- | --- |
| Go vet, gofmt, size gate, staticcheck 2026.1 | Pass |
| Go suite + `TRACEPILOT_INTEGRATION_TEST=1` | Pass trên PostgreSQL 17 disposable (Docker, loopback 55432, tmpfs); migrate up version 1 dirty=false; container đã xóa |

Không chạy trong đợt này: `-race` (máy Windows CGO=0), Playwright E2E, load test
error storm để đo trước/sau, govulncheck. Không đổi schema, REST hay frontend.
Để lại có chủ đích: tối ưu `ListTraces` (lượt quét thứ hai không có điều kiện
thời gian nên không bỏ qua partition nào; phân trang sau khi tổng hợp toàn bộ)
và giới hạn số span của `GetTrace`. Cả hai cần EXPLAIN trên dữ liệu đại diện;
giới hạn span còn đổi response contract nên UI phải báo trace bị cắt.

## Đợt SQL qua sqlc (24/10/2026)

Mục tiêu: SQL nằm trong `backend/db/queries`, được sqlc kiểm với schema; Go chỉ
gọi hàm có kiểu. Trước đợt này khoảng 76 chỗ gọi SQL viết tay, sqlc chỉ quản 3 query.

| Thay đổi | Ghi chú |
| --- | --- |
| 10 file query theo store; sqlc.yaml ánh xạ uuid→string, timestamptz→time.Time, cột nullable→con trỏ, rename ID/URL/HTTP | Bỏ `pgtype` khỏi store; helper `pgvalue.OrNil`/`TimeOrNil`/... thay các hàm trả `any` |
| `pgbatch.Queue`: DBTX cho sqlc chỉ xếp `:exec` vào một `pgx.Batch` | Ingest và ghi occurrence giữ một round trip; `Query`/`QueryRow` trong batch trả lỗi |
| Migration 000002: `span_is_dependency`, `span_dependency_type`, `span_dependency_name` (SQL, IMMUTABLE) | Thay ba hằng SQL ghép chuỗi; CurrentVersion=2; down 1 chỉ gỡ hàm |
| Architecture test chặn chuỗi SQL trong Exec/Query/QueryRow/Queue của storage | Ngoại lệ có lý do: schema_migrations, DDL partition (tên bảng quote bằng `pgx.Identifier`), savepoint |
| Sửa kèm: claim webhook giải mã secret trước commit; dropped count uint32 bão hòa ở int32 thay vì lỗi mã hóa bị retry mãi | Hai câu UPDATE webhook gộp bằng `COALESCE(sqlc.narg(enabled), enabled)` |

| Kiểm tra | Kết quả |
| --- | --- |
| sqlc 1.31.1 (Docker) | Sinh lại hai lần, hash không đổi |
| Go vet, gofmt, size gate, staticcheck 2026.1 | Pass |
| Go suite + integration trên PostgreSQL 17 disposable | Pass trên schema version 2; thêm integration cho topology/detection dependency (database, queue, external) và unit cho `pgbatch`, `pgvalue` |
| Migration roundtrip | up → down 1 (3 hàm bị gỡ) → up (3 hàm có lại), version 2 dirty=false |
| Benchmark ingest 256 log/batch, xen kẽ HEAD và bản mới trên DB mới | HEAD 65–110 ms/batch, bản mới 75–80 ms/batch; một vòng cả hai chậm ~10 lần do máy. Không thấy chênh lệch nhất quán; máy Windows + Docker không đủ ổn định để kết luận ở mức 10% |

Chưa chạy: benchmark trên Linux/staging, `-race`, Playwright E2E. Deploy bản này
cần `migrate up` lên version 2; rollback về image cũ cần `migrate down 1`.

## Đợt gọn handler REST (24/10/2026)

| Thay đổi | Ghi chú |
| --- | --- |
| Router xác thực theo scope khai báo trong bảng route; handler nhận `auth.Principal` | Bỏ 33 khối auth lặp; test mới kiểm mọi route trả 401 khi thiếu key, 403 khi sai scope, trước khi handler chạy (đã thử bỏ kiểm tra để xác nhận test bắt lỗi) |
| Helper `writeInternalError`/`writeInvalidQuery`/`writeNotFound`/`writePage`, `loadIncident`, `writeIncidentPage`, `analysisRequest`, `webhookTarget` | Mã lỗi, thông báo, thứ tự kiểm tra và response giữ nguyên |
| Sửa kèm: lỗi 500 của webhook/API key và lỗi xác thực giờ được ghi log nguyên nhân | Trước đây trả 500 mà không log |
| Sửa kèm: danh sách webhook và delivery báo đúng lỗi phân trang | Trước đây mọi lỗi (kể cả limit sai) đều báo "cursor is invalid" |

| Kiểm tra | Kết quả |
| --- | --- |
| Go vet, gofmt, staticcheck 2026.1 | Pass |
| Go suite + integration (PostgreSQL 17 disposable, schema version 2) | Pass |
| Playwright E2E trên binary thật + DB đã migrate và seed | 8/8 pass |

## Đợt quyết định production (24/10/2026)

Sáu quyết định A–F của chủ repo, đối chiếu với code và kiểm chứng. Chỉ liệt kê hàm/cơ chế chính.

| Quyết định | Thay đổi chính | Trạng thái |
| --- | --- | --- |
| A. Durability/ACK | `MetricsPipeline.SubmitMetrics` chờ commit; `StoreMetrics` phân loại lỗi nội dung | Xong: OTLP metrics ACK sau commit, lỗi DB → 503 để gửi lại. Không tuyên bố exactly-once |
| B. Retention | `retention.Run` theo nhóm (`Policy`); `maintenance.DeleteIncidentsBefore/AuditBefore/DeliveriesBefore` | Xong: 7/90/180/30 ngày, cấu hình được; chỉ xóa incident đã resolve/ignore; không orphan (cascade) |
| C. Nhiều replica | Test đa replica; sửa `ON CONFLICT` (3 câu) và provisioning khởi động | Xong: 2 lỗi thật đã sửa (bên dưới). Rate limit vẫn theo từng tiến trình |
| D. Proxy/revocation | `http.trusted_proxies`, `Server.clientAddress` | Xong: mặc định không tin header. Không có cache xác thực nên revoke có hiệu lực ngay; admin fail closed |
| E. Giới hạn trace | `GetTrace` phân trang keyset 500 span/8 MiB, `loadTrace` (UI) tối đa 5.000 | Xong: có `next_cursor`, UI báo khi cắt. Đổi hợp đồng tương thích (trường thêm vào) |
| F. Baseline/SLO | `scripts/loadgen`, metric pool DB, cổng truy vấn API | Đã đo; **REST chưa đạt mục tiêu**, xem bên dưới |

### Lỗi thật phát hiện và đã sửa

| # | Lỗi | Bằng chứng | Sửa |
| --- | --- | --- | --- |
| 14 | `EnsureService`, `InsertSpan`, `EnqueueWebhookDelivery` dùng `ON CONFLICT (khóa hẹp)` trên bảng có nhiều unique. Hai kết nối chèn cùng dòng cùng lúc → lỗi 23505 thay vì bỏ qua (detection của hai replica cùng cửa sổ; retry trùng) | Test stress thất bại ngay vòng đầu cả 3 câu; test replica thất bại 3/20 lần | `ON CONFLICT DO NOTHING` không đích. Sau sửa: stress pass, 0/30 lần thất bại |
| 15 | `EnsureIngestionScope`, `EnsureBootstrapAPIKey`: hai replica khởi động cùng lúc đụng `projects_slug_key`/`api_keys_key_hash_key` → tiến trình thoát | Test khởi động thất bại vòng 1 | Advisory lock theo transaction cho từng bước; 5 lần x 60 vòng x 4 replica pass |
| 16 | Đọc REST không bị chặn: 100 req/s làm pool DB kẹt 20/20 và ingest lỗi 99% | Đo trực tiếp, bên dưới | `admitQuery` (4 slot, 503 + Retry-After) và deadline 20s (504) |
| 17 | `GetTrace` tải cả trace vào bộ nhớ | Đọc code | Phân trang, giới hạn dòng và byte |

### Baseline đo được (không phải SLO đã cam kết)

Môi trường: Windows 11, i7-10510U (4 nhân/8 luồng, laptop), PostgreSQL 17 trong Docker (dữ liệu trên tmpfs), server, loadgen và PostgreSQL **chung một máy**, OTLP JSON, 2% span lỗi, mặc định trừ rate limit nâng lên. Mỗi lần 30s đo + 5s khởi động nóng. Số liệu nhiễu giữa các lần chạy; không dùng để suy ra năng lực production.

| Kịch bản | Kết quả | Mục tiêu |
| --- | --- | --- |
| Ingest 1.000 span/s, 20 span/request | p50 198–244 ms, p95 304–978 ms, p99 585–1.687 ms; 503: 0,5–0,7% (một lần 10%) | p95 <500, p99 <1.000 ms: đạt 3/4 lần |
| Ingest 1.000 span/s, 100 span/request (điển hình hơn) | p50 255–277, p95 452/513/877, p99 728/715/1.082 ms; lỗi 0,29/0/0% | đạt 2/3 lần (p95), 2/3 lần (p99) |
| Ingest như trên, `flush_interval=50ms` | p50 119–132 ms (giảm một nửa); p95 443/297/1.194; p99 931/585/2.270 | đuôi vẫn nhiễu; không đổi mặc định |
| Tài nguyên khi ingest 1.000 span/s | server ~17–21% một nhân, RSS 33–40 MiB; PostgreSQL 94–199% một nhân; queue tối đa 376; pool DB ≤4 (một lần 18/20) | — |
| Đọc REST 100 req/s, **chưa có cổng** | 86% request timeout 30s; pool 20/20; PostgreSQL ~1.286% CPU | p95 <200, p99 <500 ms: **không đạt** |
| Từng endpoint trên 136k span, 1 request | topology 2,1 s, danh sách trace 0,9 s, performance 0,46 s, services 0,46 s, incidents 31 ms, chi tiết trace 35 ms | — |
| Hỗn hợp 1.000 span/s + 100 req/s, **chưa có cổng** | ingest lỗi 99%; detection 45,6 s | — |
| Hỗn hợp, có cổng 2/4/8 slot | ingest lỗi 18% / 18–60% / 48%; đọc bị từ chối 96–99% | ingest được bảo vệ một phần |

Nút cổ chai đã xác minh (plan `auto_explain`): các endpoint tổng hợp quét tuần tự cả partition của ngày (136k hàng: 0,5–2,9 s), nên chi phí tăng tuyến tính theo khối lượng trong cửa sổ. Ở 1.000 span/s cửa sổ 15 phút là ~900k hàng, và 100 req/s loại này cần hàng chục nhân. **Không thể đạt mục tiêu REST bằng chỉnh index hay cấu hình**; cần rollup/bảng tổng hợp (ROADMAP P1) hoặc hạ mục tiêu cho các endpoint tổng hợp. Chưa đo được tác động của cổng khi PostgreSQL ở máy riêng. Cổng giảm sập nhưng chưa đủ: PostgreSQL vẫn bão hòa CPU trên máy dùng chung.

| Kiểm tra | Kết quả |
| --- | --- |
| gofmt, vet, staticcheck 2026.1, size gate, sqlc (sinh lại hash không đổi) | Pass |
| Go suite + integration (PostgreSQL 17 disposable, schema 2) | Pass |
| `go test -race` trên Linux (golang:1.26, kèm integration) | Pass, không race |
| Frontend: typecheck, vitest 16 file / 64 test, build assets | Pass |
| Playwright E2E trên binary thật + DB seed | 8/8 |
| Không chạy | Soak dài, tải gRPC/protobuf, PostgreSQL tách máy, Linux staging, CI trên commit release, backup/restore |

## Chưa kiểm chứng

Linux race detector (máy Windows hiện CGO=0), container image build/runtime,
CI trên commit release, full backup/PITR restore, certificate/CA rotation,
shutdown dưới tải lớn, soak/mixed gRPC-protobuf-JSON/RSS, capacity/disk/WAL,
nhiều replica và chất lượng RCA trên dữ liệu thật.

Verdict: cấu trúc và luồng chính đã có regression/integration/browser evidence;
cần đóng policy dữ liệu, tương thích DB cũ nếu có, và staging acceptance trước
khi mở production traffic.

Lean: docs từ 10 còn 5; không thêm dependency vào Go module hoặc web package.
Các bước tiếp theo và phương pháp lựa chọn ở [ROADMAP.md](ROADMAP.md).
