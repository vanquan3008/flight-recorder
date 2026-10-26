# Phương án phát triển tiếp

Cập nhật 14/10/2026. Đã chọn một ứng dụng, chia module theo trách nhiệm.
Mỗi phạm vi mới trình bày phương pháp, đánh đổi và đề xuất trước khi triển khai.
Khi đã được chọn, hoàn thành phạm vi đó mà không xin xác nhận lại từng bước.
Trạng thái và lỗi cụ thể ở [REVIEW.md](REVIEW.md).

## Phạm vi đã chốt với chủ repo

- Production nội bộ một công ty, một instance Go + React embed và PostgreSQL riêng.
- Dashboard observability gọn, hiện đại, hỗ trợ sáng/tối và responsive.
- Giữ một `.env` ở root, `.env.example` và `compose.yaml`; PostgreSQL local qua profile `local`.
- Chỉ tách tiến trình khi có yêu cầu hoặc số đo chứng minh. Phạm vi hiện tại chưa cần kiến trúc nhiều khách hàng.

Kiến trúc và luồng chính đi đúng hướng, nhưng chưa đủ bằng chứng để kết luận
production-ready. Trace, log và OTLP metrics đều ACK sau commit (metrics từ 24/10/2026).
Peak tải, SLO, RPO/RTO và tình trạng
DB cũ còn cần chốt trước nghiệm thu; retention đã chốt 7/90/180/30 ngày.

## Thứ tự thực hiện và điều kiện hoàn thành

| Đợt | Phạm vi | Điều kiện hoàn thành |
| --- | --- | --- |
| 1 — Baseline release | Đồng bộ React source với embedded assets; chốt peak tải, SLO, metrics durability, retention và DB cũ. | Build tái lập, assets khớp source; có tiêu chí định lượng cho tải, mất dữ liệu và phục hồi. |
| 2 — Dữ liệu và workflow | Redaction, retention, incident recurrence; đưa điều phối detection về application. | Regression nhỏ theo từng lỗi; PostgreSQL integration pass trên DB disposable; không đổi contract âm thầm. |
| 3 — React console | Theme sáng/tối, bộ component chung, giữ ngữ cảnh điều tra, log explorer và quản trị cần thiết. | Duyệt các màn chính với dữ liệu thật đại diện và các trạng thái rỗng/lỗi; typecheck, tests, browser checks và embedded build pass. |
| 4 — Staging acceptance | Tải hỗn hợp HTTP/gRPC, ramp/soak, DB outage/retry, shutdown, webhook retry, backup/restore và TLS. | p95/p99 đạt budget đã chốt; đối chiếu ACK với rows lưu; giới hạn mất metrics đúng contract; phục hồi đạt RPO/RTO. |
| 5 — Release gate | CI trên commit release, image runtime, runbook và rollback tương thích schema. | Không còn lỗi P0; các giới hạn còn lại được ghi rõ và chấp nhận trước khi mở traffic. |

Các đợt trên là kế hoạch, chưa phải kết quả nghiệm thu. Checklist vận hành chi
tiết tiếp tục nằm ở [OPERATIONS.md](OPERATIONS.md), không tạo thêm báo cáo riêng.

## Ranh giới module cần giữ khi triển khai

| Module | Trách nhiệm và việc cần hoàn thiện |
| --- | --- |
| `cmd/tracepilot` | Config, wiring, startup/shutdown; không thêm quy tắc nghiệp vụ vào CLI. |
| `internal/domain` | Model, fingerprint, status transition, detection policy và RCA ranking thuần; test không cần DB/runtime. |
| `internal/application` | Điều phối incident, intelligence, notification, retention và replay qua interface nhỏ gần consumer. Việc gọi ba nhóm detector, gom evidence theo service và điều phối lưu nằm ở `application/detection`; `storage/postgres/analytics` chỉ còn SQL và transaction. |
| `internal/ingest` | OTLP HTTP/gRPC, validation, normalization, redaction, admission, queue và ACK; giữ regression retry, partial rejection và shutdown. |
| `internal/transport/httpapi` | Auth/scope, parse request, response/error contract và OpenAPI; server vẫn quyết định quyền truy cập. |
| `internal/storage` | SQL, transaction, migration, partition và persistence; giữ atomic event/incident/evidence/outbox. |
| `web/src/app` | Router, session, shell và điều khiển theme; làm rõ kết nối thất bại/hết quyền. |
| `web/src/features` | Giữ overview, performance, traces, operations và topology theo chức năng; thêm logs và settings khi triển khai các màn tương ứng. Feature sở hữu truy vấn và component của nó. |
| `web/src/shared` | API client, time window, formatting và UI dùng chung thực sự; không import ngược vào features. |

Giữ architecture test backend. Bổ sung kiểm tra import frontend khi chia module
để ngăn phụ thuộc vòng và shared phụ thuộc feature. Chỉ tách file theo trách
nhiệm, không bọc mọi query bằng một lớp service chuyển tiếp.

## Tiến độ React console (14/10/2026)

Đã làm trong source, đã build lại embedded assets; chưa phải nghiệm thu đợt 3.

- Theme sáng/tối bằng semantic tokens, mặc định theo hệ thống, lưu `localStorage`;
  `theme-init.js` chạy trước first paint nên không chớp theme sai.
- Logs explorer (`/logs`): filter service/severity/body/trace/span, window, body dài
  thu gọn, link trace; từ span mở log đúng span.
- Settings (`/settings`): API key tạo/rotate/revoke có xác nhận, secret hiển thị một
  lần; webhook tạo/bật-tắt/rotate/xóa và delivery history; 401/403/501 có thông báo riêng.
- Topology: graph phân tầng, node chọn được, node không có edge tách riêng, chi tiết
  calls out/called by và link trace; vẫn giữ bảng edge.
- Ngữ cảnh điều tra: Back từ trace/incident về đúng list với filter và time window.
- Overview ưu tiên incident cần xử lý; service trong bảng performance dẫn sang traces.
- Trace: trace hết retention có hướng xử lý, cảnh báo span thiếu parent, clipboard báo lỗi.
- Trace detail xem toàn cảnh: tóm tắt (thời lượng, span, service, lỗi), flow service → service của
  riêng trace, thời gian tự thân theo service, waterfall có trục thời gian, thanh theo màu service,
  lỗi có chữ + sọc (không chỉ màu), thu gọn/mở nhánh, chi tiết span ngay dưới dòng, bấm service để
  làm nổi span. Graph dùng chung với Topology (`shared/service-graph`).
- `architecture.test.ts` chặn shared → app/features, features → app (trừ session),
  feature ↔ feature ngoài danh sách cho phép, và import vòng.

Đã làm thêm: điều phối detection (ba nhóm detector, gom evidence theo service, lưu incident) nằm ở
`internal/application/detection`; storage chỉ còn SQL/transaction. Playwright E2E 7/7 trên Go + PostgreSQL.

Còn lại của đợt 3: duyệt màn với dữ liệu thật đại diện và manual accessibility (zoom 200%,
screen reader, contrast).

Đợt chất lượng tín hiệu ingest (24/10/2026, chi tiết ở REVIEW #11–13): fingerprint chuẩn hóa
message, ghi incident gom theo fingerprint, cô lập event PostgreSQL từ chối khỏi batch. Tiếp
theo, theo thứ tự: load test kịch bản error storm để đo trước/sau; EXPLAIN `ListTraces` trên dữ
liệu đại diện rồi thêm điều kiện thời gian cho lượt quét thứ hai. Đã làm: `GetTrace` phân
trang (500 span/trang, UI đọc tối đa 5.000 và báo khi cắt).

Đã chốt và triển khai (REVIEW, "Đợt quyết định production"): OTLP metrics ACK sau commit;
retention 7/90/180/30 ngày; trusted proxy cấu hình được; trace phân trang; test đa replica;
`scripts/loadgen` và baseline ban đầu.

Còn chờ chủ repo chốt, chưa triển khai: incident reopen (cần migration cho `actor_key_id` của
status change để ghi actor hệ thống, đổi semantics webhook), server-side redaction free-text
(cần policy/fixture), DB cũ, CIDR proxy của hạ tầng thật, mục tiêu REST cho endpoint tổng hợp.

### Việc tiếp theo theo ưu tiên

1. **Rollup cho endpoint tổng hợp** (services, topology, danh sách trace, performance): baseline cho thấy
   chúng quét cả cửa sổ span nên không đạt p95 <200 ms ở 136k span. Cần quyết định: bảng tổng hợp theo
   phút do job nền cập nhật (tăng độ trễ dữ liệu), hoặc chấp nhận mục tiêu cao hơn cho nhóm này.
2. Đo lại trên Linux/staging với PostgreSQL tách máy: kết quả cổng truy vấn, `flush_interval` 50 ms,
   `max_concurrent_requests` của OTLP (503 khi ACK chậm đột ngột), pool DB; chốt SLO thật.
3. Metric cho outbox (số chờ, tuổi lâu nhất) và thời gian chờ trong queue ingest.
4. Giới hạn stream đồng thời của gRPC; cân nhắc cache xác thực ngắn hạn (revoke ≤10s) nếu số đo cho thấy
   truy vấn xác thực là điểm nghẽn (chưa có bằng chứng).
5. Rate limit dùng chung nhiều replica (hiện theo từng tiến trình): ưu tiên đặt ở reverse proxy.

## Nền tảng đã có trước kế hoạch này

- Gom replay/notification/intelligence/retention vào application, cập nhật callers.
- Tăng architecture guard cho domain, application, transport, ingest và storage.
- Giới hạn byte metrics cả queue và batch đang ghi; test success/failure.
- Load tool tạo record riêng mỗi request, đếm partial rejection, có self-check/CI.
- Partition maintenance không nuốt lỗi quyền/disk/connection; có regression test.
- Chuyển tiếp rate limit/retention/queue budget từ production env vào container.
- Thay tài liệu cũ bằng architecture, development, operations, review, roadmap.
- Gom cấu hình còn compose.yaml và .env ở root; chỉ giữ một .env.example.
  Profile local bật DB development; production dùng DB riêng và cùng Compose.

## P0 — Xác minh trước khi chạy production

Traffic, performance và capacity là ba mục riêng:

| Mục | Hiện đã có | Còn thiếu |
| --- | --- | --- |
| Traffic backend được theo dõi | `/api/v1/performance` và React source đã có observed requests, RPS, error rate, breakdown theo service/endpoint và trend từ SERVER spans. `/metrics` đo chính TracePilot. | Đồng bộ embedded assets; đối chiếu SDK/Collector đại diện. Khi có sampling/mất telemetry, số quan sát không phải tổng traffic thật. |
| Performance | API/source UI đã có average, p95/p99 của inbound requests trong cùng cửa sổ; có regression cho filter, scope, idle bucket và aggregate. | Đo query trên dữ liệu/cardinality đại diện, chạy lại integration trên DB test; chốt budget và kiểm tra sampling bias. |
| Capacity | Admission OTLP mặc định 32 request, queue có budget, load script có concurrency/latency/errors. | Ramp/stress/soak tìm mức đồng thời tối đa còn đạt SLO trên backend mục tiêu. Script hiện chỉ gửi OTLP HTTP logs; giới hạn cấu hình không phải capacity đo được. |

Dashboard RED (rate, errors, duration) đã có trong source; không lập lại như một
tính năng chưa tồn tại. Hai hướng đo tiếp: (A) đối chiếu received SERVER spans
với dữ liệu đầu vào; (B) dùng counter/histogram đáng tin từ backend mục tiêu khi
cần tổng traffic dù trace bị sampling. Đề xuất A cho baseline hiện tại, sau đó
benchmark tăng dần concurrency; chọn B khi yêu cầu SLO cần nguồn đo đầy đủ hơn.

| Việc | Các phương pháp | Đề xuất và điều kiện xong |
| --- | --- | --- |
| Embedded assets | Build Vite rồi Go embed; kiểm tra generated diff trong CI | Bundle hiện có chưa chứa dashboard performance của React source. Build lại trước release; truy cập binary/image thật phải thấy cùng chức năng với source build. |
| Test DB/runtime | Docker Compose disposable; PostgreSQL native disposable; CI Linux | CI Linux + staging Compose, vì đồng thời kiểm tra race và image. Baseline roundtrip, integration, browser smoke, outage/restore/shutdown có evidence trên commit release. |
| DB cũ | Giữ binary/schema cũ; viết đường data migration có kiểm chứng; tạo DB mới nếu dữ liệu có thể bỏ | Hỏi rõ dữ liệu đang có trước. Không tự recreate DB. Dataset và row counts trước/sau phải khớp. |
| Tải/SLO | Một replica và đo; tách ingest/worker/API khi bottleneck rõ | Đo một replica trước. Chốt peak, p95/p99, RSS, disk và retention trên dữ liệu/cardinality đại diện. |
| Lọc secret | SDK/Collector processors; bổ sung server redaction theo policy/fixtures | Làm ở SDK/Collector trước và dùng fixture kiểm chứng DB/evidence/webhook không lộ secret. Free-text cần policy cụ thể. |
| PostgreSQL TLS | verify-full + trusted CA; require khi chỉ cần encryption và chấp nhận giới hạn | verify-full cho production; chốt CA/hostname, kiểm tra connection sai CA/DNS bị từ chối. |
| Retention | TTL từng nhóm; archive trước TTL; giữ vô hạn với capacity plan rõ | Chốt riêng telemetry/incident/evidence/audit/deliveries, không tự đặt TTL gây mất dữ liệu. Đo tăng trưởng và restore. |
| Metrics contract | Chấp nhận best-effort có giới hạn rõ; chờ commit nếu không cho mất sau ACK | Chốt trước release. Nếu metrics là nguồn duy nhất cho SLO/cảnh báo quan trọng, phải có mức mất dữ liệu được chấp nhận hoặc cơ chế durability tương ứng. |

## P1 — Hoàn thiện cơ chế đang có

| Phạm vi | Phương pháp A | Phương pháp B | Đề xuất |
| --- | --- | --- | --- |
| Metrics durability | Chờ DB commit như trace/log | Collector persistent queue hoặc durable broker và worker | A nếu yêu cầu không mất sau ACK; Collector queue hỗ trợ mất kết nối nhưng không tự sửa việc server ACK trước storage. Chọn sau khi chốt SLO và benchmark. |
| Incident recurrence | Giữ dedup hiện tại và tạo incident mới sau cửa sổ | Reopen khi occurrence thực sự mới hơn lúc resolve, kèm audit/outbox | B nếu workflow on-call cần reopen; delayed event không reopen, IGNORED phải có policy riêng. |
| Query/detection performance | Profile/EXPLAIN, index có số đo | Rollup hoặc job riêng | A trước; B khi raw queries vượt budget. Giữ scope, dedup, late-arrival tests. |
| Partition default backlog | Giữ fallback và batch retention | Job chuyển dữ liệu sang dated partition | A với lưu lượng nhỏ; B khi default partition tăng hoặc tạo partition bị chặn kéo dài. Cần đo lock/downtime. |

## P1 — React UI và trải nghiệm điều tra

Hai phương pháp thực tế: (A) hoàn thiện CSS variables và component đang có;
(B) đưa vào một UI library rồi chuyển các màn sang library đó. Đề xuất A vì
repo đã có nền tảng phù hợp, ít thay đổi và không cần thêm dependency để hỗ
trợ theme/responsive. Đánh đổi là repo tự duy trì component và accessibility.

| Phạm vi | Cách triển khai đề xuất | Điều kiện nhận |
| --- | --- | --- |
| Theme sáng/tối | Semantic tokens cho nền, chữ, border, trạng thái và chart; mặc định theo hệ thống, cho đổi thủ công và lưu lựa chọn trên trình duyệt. Thay màu hardcoded còn trong CSS. | Cả hai theme đọc rõ, không chớp theme sai khi mở trang; lựa chọn tồn tại sau reload; button, table, chart và error states cùng theme. |
| Thiết kế chung | Thống nhất typography, spacing, button, input, badge, panel và table bằng component/tokens hiện có. | Dữ liệu quan trọng rõ thứ bậc; trạng thái không chỉ phân biệt bằng màu; không thêm trang trí cản việc đọc số liệu. |
| Overview/performance | Ưu tiên traffic, error, latency và incident cần xử lý; thu gọn onboarding sau hoàn tất; ghi rõ số tổng và số của trang hiện tại. | Cùng time window và filter; loading/error/empty rõ; service/endpoint dẫn được sang dữ liệu điều tra liên quan. |
| Ngữ cảnh điều tra | Giữ time window, service, status và vị trí danh sách khi mở detail rồi quay lại. | Browser regression cho overview → trace list → detail → back và incident list → detail → back. Hiện nút Back to traces/incidents làm mất query filters. |
| Traces và logs | Feature traces sở hữu list/waterfall/span detail; thêm feature logs dùng API hiện có, filter và correlation trace/span. | Đọc được log độc lập, log body dài, JSON, trace thiếu span và trace hết retention; lỗi clipboard có phản hồi. |
| Topology | Graph chọn node với nhãn service/dependency dễ đọc, chi tiết và link trace; giữ bảng bổ sung. | Test isolated service, nhiều node, nhãn dài, không có edge và chuyển sang trace; đánh giá hiệu năng trên số node đại diện. |
| Settings | API keys/scopes/rotation/revoke; webhook config và delivery history khi bật notifications. | UI phản ánh quyền đọc/ghi/admin và lỗi 401/403; secret hiển thị theo contract một lần; mutation có pending/error rõ. |
| Responsive/accessibility | Kiểm tra viewport 390, 768 và 1440px, zoom 200%, keyboard/focus, label, contrast và reduced motion. Mục tiêu WCAG 2.2 AA theo [W3C](https://www.w3.org/WAI/standards-guidelines/wcag/). | Không tràn ngang toàn trang; bảng rộng có vùng cuộn rõ; mobile ưu tiên KPI chính. Screenshot review cả sáng/tối và các trạng thái lỗi/rỗng/nội dung dài; automation không thay manual accessibility review. |

Chỉ xem React UI hoàn thành khi typecheck, tests, browser regression, visual
review và build lại embedded assets đều đạt. Render fixture trên browser chỉ
xác minh trình bày/tương tác; E2E trên Go + PostgreSQL test phải kiểm tra dữ
liệu và quyền thật. Cập nhật kết quả thực chạy vào REVIEW, không gọi unit test
là chứng nhận production.

## P2 — Sản phẩm mới, sau khi P0/P1 có evidence

| Phạm vi | Phương pháp | Điều kiện nhận |
| --- | --- | --- |
| Threshold/SLO theo service | Config đơn giản; UI quản lý policy khi có nhiều operator | Dataset đo false positives/detection delay và warm-up. |
| RCA nhiều service | Evidence + topology + time; thêm deployment/change events | Không suy ra causality chỉ từ thứ tự signal; output tham chiếu evidence thật. |
| LLM narrative | Template deterministic; LLM tùy chọn tóm tắt evidence IDs | Timeout/fallback/eval, không bịa evidence hoặc tự remediation. |
| Scale riêng | Giữ monolith; tách tiến trình từ module có bottleneck | Số đo throughput/RSS/DB contention chứng minh nhu cầu và có kế hoạch vận hành. |

Bước đề xuất tiếp theo là đợt 1: đồng bộ assets và chốt các tiêu chí production;
sau đó hoàn thiện dữ liệu/workflow và React console theo phạm vi đã chọn.
Chưa chọn infrastructure mới, broker, graph/vector database hoặc LLM.
