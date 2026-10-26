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

### 9. Chính sách lưu chưa giới hạn toàn bộ database

- Là gì: retention.max_age mặc định 0; worker chỉ chạy khi > 0.
  Incident/evidence/audit/webhook delivery history chưa có TTL riêng.
- Vấn đề: mặc định telemetry tăng mãi; ngay cả khi đặt telemetry TTL, các bảng
  điều tra/audit vẫn tăng. Trace được incident tham chiếu có thể đã hết retention.
- Cách xử lý: chốt retention/archive từng nhóm và budget đĩa; triển khai TTL
  sau khi được chọn, có test cutoff và audit policy. Compose đã cho truyền max_age.
- Nếu bỏ qua: có thể hết đĩa và ingestion thất bại. Phiên này giữ 0s để không tự
  quyết định xóa dữ liệu theo một thời hạn chưa được người dùng chọn.

## Nên hoàn thiện

### 10. Đóng incident chưa có quy tắc reopen theo occurrence mới

- Là gì: storeIncident tìm cùng fingerprint trong cửa sổ một giờ, không lọc
  status khi thêm occurrence.
- Vấn đề: lỗi mới trong cửa sổ có thể được thêm vào incident RESOLVED mà status
  vẫn RESOLVED; webhook incident.opened không được phát lại.
- Cách xử lý: chọn giữ dedup hiện tại hoặc reopen khi occurrence mới hơn thời
  điểm resolve, kèm actor/audit/outbox; delayed event và IGNORED có rule riêng.
- Nếu bỏ qua: operator cần biết resolved không đồng nghĩa không còn lỗi.
  Chưa đổi semantics khi chủ repo chưa chọn policy.

## Các giới hạn được giữ có chủ đích

- Metrics vẫn ACK trước DB write. Byte budget giúp kiểm soát admission, không
  thay cơ chế best-effort bằng durability. Không dùng làm nguồn SLO duy nhất
  nếu chưa chấp nhận mất point hoặc chọn phương pháp durability.
- Hai pipeline có budget riêng, không phải giới hạn tổng RSS. gRPC decode trước
  admission; cần load/RSS measurement trên payload và cardinality đại diện.
- Một server phục vụ environment cấu hình; query đã scope environment nhưng
  chưa có UX chuyển environment hay cam kết scale nhiều replica.
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
