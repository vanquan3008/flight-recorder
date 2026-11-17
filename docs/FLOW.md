# Luồng hoạt động của TracePilot

Tra cứu vai trò từng file, chữ ký hàm, ví dụ sử dụng và chuỗi gọi tại [bản đồ source HTML](diagrams.html#reading-guide); [mục lục file/hàm](diagrams.html#source-map) có tìm kiếm và bộ lọc test/mã sinh.

Mô tả các luồng chính của hệ thống. Sơ đồ tương ứng ở [diagrams.html](diagrams.html)
(mở bằng trình duyệt, cần mạng để tải thư viện vẽ). Số trong ngoặc `[n]` là số sơ đồ.

Một ứng dụng Go (API quản trị + bộ thu OTLP + worker nền), PostgreSQL riêng,
ClickHouse bắt buộc cho raw log và console React riêng.

Ranh giới source: `cmd/tracepilot` nối dependency và quản lý vòng đời; `application` điều phối use case
qua interface nhỏ; `domain` giữ quy tắc thuần; `ingest`, `transport/httpapi` và `storage` giữ chi tiết
giao thức/lưu trữ. `application/session` xác minh credential và đổi mật khẩu; `application/live` quản lý
hub telemetry trong bộ nhớ. Frontend lấy context đăng nhập từ `shared/session`; `app` quản lý vòng đời
session và ghép route, các feature không import ngược vào `app`. Phạm vi MVP, điều kiện phát hành và
kế hoạch tiếp theo nằm trong [README](../README.md#mvp-scope-and-release-criteria).

## 1. Tổng quan [1]

- Ứng dụng của bạn gửi OTLP (gRPC `:4317`, HTTP `:4318`) kèm **ingest token**.
- Bộ thu xác thực, chuẩn hóa, đưa vào **pipeline** có giới hạn; pipeline ghi batch
  theo scope. Trace/metric vào PostgreSQL, 100% raw log vào ClickHouse (kho bắt buộc cho log storage).
- Log thường chỉ ghi ClickHouse; log gây incident giữ envelope lỗi gọn trong PostgreSQL.
  Incident/evidence/occurrence và outbox vẫn commit cùng transaction PostgreSQL.
  ACK chờ ClickHouse flush và mọi transaction PostgreSQL cần thiết thành công.
- Worker nền: phát hiện bất thường mỗi phút, gửi webhook, tạo partition, dọn dữ liệu hết hạn.
- Người dùng vào **console**, console gọi REST `/api/v1` (`:8080`) bằng access token.
- Console giữ project/environment trên URL và gửi chúng bằng `X-Project-ID`/`X-Environment`.
  Dropdown nổi dùng chung cho Project/Environment: điều hướng bằng bàn phím hoặc con trỏ,
  chỉ xác nhận mới đổi scope; Esc, Tab hoặc bấm bên ngoài đóng danh sách. Menu bám nút và giới hạn trong viewport.
  Chuyển scope xóa query cache và dựng lại trang; buffer Live Tail của scope cũ không được giữ lại.
  Liên kết giữa các trang giữ scope. Điều hướng các mục giữ khung thời gian; drill-down có
  `from/to` dùng nguyên khoảng tuyệt đối, kể cả sau khi tìm kiếm hoặc bấm Back.

## 2. Khởi động và tắt [2]

### Triển khai container

`deploy/compose.yaml` chỉ dựng ứng dụng; PostgreSQL và ClickHouse là dịch vụ bên ngoài.
Compose đọc `backend/.env` và `frontend/.env` theo đường dẫn tương đối từ `deploy/`;
`certs/` phải có sẵn và chỉ chứa public CA. Service `migrate` chạy `migrate all` bằng cùng
image với backend, không dùng healthcheck của server. Backend chỉ chạy sau exit code 0;
console chỉ chạy khi backend healthy. `up --wait` chờ các điều kiện này trước khi trả thành công.

Stack buộc production validation và tắt hai cờ auto-migrate ở `serve`, tránh mỗi lần start
server lại tự quản lý schema. Tên container/network theo Compose project; DNS service giữ nguyên.
Port chỉ bind loopback, HTTPS do nginx trên host xử lý. Filesystem read-only, tmpfs dành cho
nginx render config và file tạm; container dừng trong grace 60s. Khi nâng cấp: dừng exporter,
dừng/drain backend, sao lưu DB rồi chạy migration của image mới trước khi khởi động app.
Dependency startup không thay cho readiness/retry khi DB lỗi trong lúc hệ thống đang chạy.

### Vòng đời backend

Khởi động theo thứ tự, bước nào lỗi thì thoát với mã 1:

Trước khi chạy bản mới, bước cập nhật chạy `migrate all`: nâng PostgreSQL (khóa advisory riêng)
→ nâng ClickHouse (nhật ký riêng, không cần kết nối PG khi chạy lệnh CH riêng) → kiểm readiness
cả hai. Bước nào lỗi thì không khởi động app; không tự rollback bước đã xong vì hai DB không
có transaction chung. `make start`/`make dev` nối bước này với `serve`, kể cả khi Make chạy song song.
`serve` dùng hai cờ độc lập trong `backend/.env`: `TRACEPILOT_DATABASE_AUTO_MIGRATE`
cho PostgreSQL, `TRACEPILOT_CLICKHOUSE_AUTO_MIGRATE` cho ClickHouse, cùng mặc định false.
Cờ nào true thì chạy bộ migration tương ứng trước khi mở listener/worker; lỗi thì dừng khởi động.
Log chỉ nằm ở ClickHouse, không có fallback sang PostgreSQL. Runtime vẫn yêu cầu cả hai DB sẵn sàng.
Lock PG chờ tối đa `DATABASE_MIGRATION_LOCK_TIMEOUT` (10s); mỗi câu migration PG tối đa
`DATABASE_MIGRATION_STATEMENT_TIMEOUT` (5 phút); kết nối migration dùng `DATABASE_CONNECT_TIMEOUT` nếu URL không có `connect_timeout`;
mỗi request CH dùng timeout cấu hình. Hủy PG giữa các file; câu đang chạy có thể đợi hết timeout.

CLI có entry point riêng: `migrate_postgres.go` và `migrate_clickhouse.go`, mỗi bên kiểm lệnh và
chỉ nạp/validate cấu hình của kho đó cùng chính sách TLS production. Giá trị thiếu hoặc sai định dạng
của kho kia và cấu hình ứng dụng không chặn lệnh riêng. Cả hai vẫn dùng một file `backend/.env`.
Auto-migrate gọi trực tiếp từng runner theo cờ của kho đó; không có dispatcher quản lý migration chung.
`storage/postgres/migrations.Run` tự giữ advisory lock cho mọi `up`/`down`, kể cả lúc tạo journal mới;
caller không tự khóa thêm. `status`/`version` chỉ đọc và không chờ khóa mutation này.
Helper CLI chỉ dùng chung việc đọc cấu hình, tín hiệu dừng và log; helper status chỉ tính target/pending
từ lịch sử được chọn, không giữ trạng thái hay version chung. Hai storage adapter không import nhau;
architecture test bảo vệ ranh giới này. Không có lệnh chuyển dữ liệu chéo kho.
`migrate all` là lệnh tiện ích gọi lần lượt hai bộ quản lý độc lập, không có journal/version riêng.
Mọi lệnh ghi log JSON qua `log/slog` (kèm `service.name`, `service.version`); `serve` áp mức `LOG_LEVEL`.
Package `config` chỉ trả lỗi, lệnh gọi log đúng một lần.
Lệnh này dùng lại luồng mở client và readiness của runtime, chỉ cần credential database,
không yêu cầu JWT secret hay khóa mã hóa webhook.

CH kiểm engine/partition/dedup key trước khi đổi TTL, kiểm quyền đọc các cột rồi mới ghi version sạch.
Migration bình thường và chạy lại chỉ kiểm schema/cột một lượt; chỉ đọc lại schema nếu đã ALTER TTL
để xác minh thay đổi. Readiness dùng chung quy tắc layout và TTL; lỗi cuối migration vẫn giữ dirty.

SQL được embed từ `backend/db/postgres/migrations` và `backend/db/clickhouse/migrations`,
mỗi DB có dãy version/lịch sử riêng, version đích lấy từ file. PostgreSQL có baseline
`000001_init.up.sql` (tạo bảng, constraint, index, partition và hàm trong cùng transaction) và
`000002_drop_postgres_logs` xóa bảng `logs` cũ; down của nó chỉ tạo lại bảng rỗng, không khôi phục dữ liệu.
Baseline không có down SQL; lệnh rollback baseline bị từ chối trước khi đổi journal hoặc xóa bảng.
Thay đổi bảng về sau thêm ID mới, tiếp theo là `000003`, không sửa file đã áp dụng.
`migrate postgres status` và
`migrate clickhouse status` chỉ đọc version/target/dirty/pending, không tạo metadata.
PG dùng `schema_migrations`; CH dùng nhật ký `tracepilot_schema_migrations`: ghi dirty
trước DDL và clean sau khi thành công (clean thắng dirty của cùng version). Lỗi/đứt kết nối
giữa chừng có thể để dirty; phải kiểm tra và sửa thủ công, không tự force hay chạy down.
CH lần đầu tiếp nhận bảng log cũ sau khi kiểm schema/TTL, giữ nguyên dữ liệu.
Lệnh từng DB chỉ yêu cầu cấu hình/kết nối
của DB đó; `all` cần cả hai. CH không có khóa migration; DDL không idempotent cần một
tiến trình cập nhật duy nhất. Baseline đã được gộp cho DB mới, không tự chuyển
lịch sử cũ hay reset DB. Khi phát hành, chỉ thêm tiếp, không sửa/gộp/đánh số lại file đã áp dụng.
Không còn cấu hình đường dẫn SQL runtime.

Luồng database-first: thêm migration SQL → cập nhật named query ở `backend/db/queries` → sinh lại
sqlc từ cùng lịch sử migration → cập nhật storage → chạy integration trên DB dùng một lần.
`db/external` chỉ mô tả metadata của công cụ migration cho sqlc, không phải một schema runtime thứ hai.
PostgreSQL tự kiểm trạng thái/severity incident và danh sách event webhook không rỗng, một chiều,
chỉ chứa event hỗ trợ; import/ghi SQL trực tiếp cũng chịu các constraint này. Migration thêm constraint
sẽ từ chối dữ liệu cũ sai quy tắc, không tự sửa/xóa dữ liệu hay bỏ dirty journal.

1. Đọc cấu hình (mặc định → `backend/.env` → biến môi trường). Sai cấu hình → mã 2.
2. Không có `TRACEPILOT_AUTH_JWT_SECRET` → sinh khóa ngẫu nhiên và cảnh báo (production
   thì cấu hình sai ngay từ bước 1).
3. Chạy auto migration theo từng cờ (nếu bật), PostgreSQL trước rồi ClickHouse. Sau đó mở một
   pool PostgreSQL dùng chung và một HTTP client ClickHouse dùng chung. Pool PG mặc định
   2–20 kết nối, connect timeout 5s (URL `connect_timeout` được ưu tiên); vòng đời kết nối có jitter
   10% để tránh thay đồng loạt. Ngân sách tối thiểu tính cả worker events và metrics, cộng hai kết nối.
4. Kiểm tra đồng thời hai kho trong thời hạn lớn hơn giữa cấu hình `DATABASE_CONNECT_TIMEOUT` và `CLICKHOUSE_REQUEST_TIMEOUT`
   (mặc định 10s): PG kiểm version đích từ SQL nhúng/không dirty qua chính pool runtime;
   CH kiểm version/không dirty, schema/TTL và quyền đọc.
   Lỗi thì hủy kiểm tra còn lại và đóng cả hai client. Nếu không bật auto migration cho DB nào,
   cần migrate DB đó trước bằng lệnh riêng hoặc `migrate all`. Sau đó tạo partition PG; không tự tạo project/environment mặc định.
   Readiness cũng kiểm hai kho đồng thời trong cùng deadline 2s; truy vấn version PG đã kiểm luôn
   kết nối nên không thêm một lần ping riêng. Không tự fallback khi ClickHouse hỏng.
5. Dựng replay (nếu bật), metrics, hai pipeline (events, metrics), bộ thu OTLP, API.
6. Chạy song song: API, OTLP (thành phần cốt lõi; lỗi hoặc dừng thì tiến trình dừng),
   cùng detection, bảo trì partition, webhook (nếu bật), retention (nếu có max_age)
   được giám sát phục hồi tự động (restart with backoff) để đảm bảo tính sẵn sàng cao.

Tắt (SIGINT/SIGTERM hoặc một thành phần lỗi): listener ngừng nhận và chờ request, rồi pipeline
events và pipeline metrics drain song song trong `shutdown_timeout` (15s), rồi đóng HTTP client CH
và pool PG. Đặt grace của container
≥ 35s. Exporter gửi lại phần chưa ACK.

## 3. Đăng ký, đăng nhập, refresh [3]

- Production cài mới dùng CLI `create-admin --email ... --name ... --password-stdin` sau migration.
  Lệnh chỉ cần PostgreSQL và vẫn kiểm TLS production/schema readiness; dùng cùng quy tắc mật khẩu
  và transaction tạo user + audit. Mật khẩu đọc một dòng từ stdin, không qua argument hay log;
  email đã có làm lệnh thất bại, không đổi quyền hoặc mật khẩu của tài khoản đó.

- **Đăng ký:** seed chỉ tạo tài khoản super admin development (`tracepilot seed` với `admin@tracepilot.local` / `admin-password-123`), không tạo project, environment, membership hay telemetry mẫu. Chạy lại seed giữ nguyên tài khoản và dữ liệu đã có. Sau khi đăng nhập, tạo project trong console. Các tài khoản khác đăng ký qua form/API khi `ALLOW_REGISTRATION=true` và khởi tạo ở quyền tài khoản thông thường. Sau đó super admin sẽ cấp quyền sử dụng: cấp cờ super admin (`platform_admin`) trong Settings → Accounts hoặc thêm vào project trong Settings → Members. Mật khẩu 12–256 ký tự. Khi `ALLOW_REGISTRATION=false`, đăng ký tự do bị đóng hoàn toàn.
- **Đăng nhập:** giới hạn theo IP và 10 lần/email/cửa sổ. Email lạ hay sai mật khẩu đều tốn cùng
  thời gian và cùng một thông báo. Thành công → server tạo phiên refresh, đặt cookie
  `tp_refresh` (`HttpOnly`, `SameSite=Strict`, path `/api/v1/auth`) và trả **access token** (JWT HS256,
  15 phút) cùng danh sách project.
- HTTP handler giải mã request, áp rate limit rồi gọi `application/session.Authenticate` qua port
  đọc credential. Use case chuẩn hóa email, xác minh mật khẩu và từ chối tài khoản bị khóa; lỗi DB
  được trả về riêng với sai credential. HTTP giữ việc cấp JWT, cookie và response contract.
- **Access token** chỉ nằm trong bộ nhớ trình duyệt. Hết hạn → API trả `TOKEN_EXPIRED` → console
  gọi `/auth/refresh` (kèm header `X-TracePilot-Refresh`) rồi gửi lại request. Mỗi lần refresh đổi
  token mới; dùng lại token cũ bị từ chối. Phiên hết hạn sau 7 ngày không dùng hoặc 30 ngày tuyệt đối.
- **Đổi mật khẩu / khóa tài khoản:** tăng `tokens_valid_after` ít nhất một microsecond trong transaction,
  dùng mốc này làm phiên bản `tp_ver` trong access JWT và đối chiếu bằng nhau ở REST/SSE. Token cũ
  mất hiệu lực cả khi cấp và thu hồi trong cùng giây hoặc đồng hồ ứng dụng chạy chậm. Refresh/đăng nhập
  đọc phiên bản hiện tại; JWT cũ thiếu `tp_ver` chỉ hợp lệ với tài khoản chưa từng thu hồi token.
  Tài khoản do admin tạo có mật khẩu tạm: chỉ gọi được
  `/auth/me` và `/auth/password` cho tới khi đổi.
- `application/session.ChangePassword` kiểm mật khẩu hiện tại, từ chối dùng lại mật khẩu và gọi
  storage để đổi cùng việc thu hồi phiên/token trong transaction; giữ session của trình duyệt hiện tại
  khi có refresh cookie. Handler đọc lại tài khoản từ DB và trả session view cùng JWT mới.
- Token mới sau đổi mật khẩu dùng cùng issuer, audience và TTL đã cấu hình như đăng nhập/refresh.

## 4. Xác thực và phân quyền một request API [4]

Chuỗi middleware (ngoài vào trong): request ID → security headers → metrics → rate limit
(theo nhóm: `query`, `auth`, `replay`) → access log → CORS → recover panic → giới hạn body → router.

Router kiểm theo **scope** mà bảng route khai báo; handler không tự kiểm quyền:

1. Thiếu/sai token → 401 (hết hạn → `TOKEN_EXPIRED`).
2. Đọc tài khoản từ DB: không có, bị khóa, hoặc token cấp trước `tokens_valid_after` → 401.
3. Đang dùng mật khẩu tạm mà không phải route cho phép → 403 `PASSWORD_CHANGE_REQUIRED`.
4. Scope `session` (mọi người đã đăng nhập) → xong. Scope `platform` → cần super admin.
5. Route theo project: lấy project từ path hoặc `X-Project-ID` (ID hoặc slug, cache bộ nhớ 30s).
   Project không có và project không được phép đều trả cùng 403 để không dò được tên.
6. Vai trò: super admin = owner ở mọi project; người khác lấy từ `project_members` (cache bộ nhớ 30s,
   xóa cache khi đổi thành viên). `user` → `read, write`; `owner` → thêm `replay, admin`. Thiếu scope → 403.
7. Resolve environment (`X-Environment`, mặc định là cái đầu tiên, cache 30s); sai → 400.
8. Xin slot truy vấn (chờ 500 ms, hết → 503), chạy dưới deadline 20 s (quá → 504). Các route xác thực
   công khai (`/auth/*`) có ngân sách slot riêng (16 slot) để không bao giờ bị nghẽn bởi truy vấn telemetry.
   Thời gian chờ và số slot active được giám sát bằng Prometheus metrics (`tracepilot_concurrency_gate_*`),
   kèm hiệu năng bộ đệm xác thực (`tracepilot_auth_cache_requests_total`).

## 5. Ingest OTLP [5]

1. Request đến HTTP hoặc gRPC; lấy `Authorization: Bearer <ingest token>` và `X-Environment` (gRPC:
   metadata `x-environment`).
2. Kiểm sổ chặn: client sai token quá `AUTH_FAILURE_LIMIT` lần/cửa sổ → 429, không truy vấn DB.
3. Xác thực: chữ ký, issuer/audience, loại `ingest`, hạn; sau đó tra `ingest_tokens` qua bộ đệm
   bộ nhớ (cache 30s, thu hồi xóa tức thì). Token sai → 401; DB lỗi → 503 để exporter retry.
4. Phân giải scope (project + environment) qua `scopeCatalog` (cache 30 s). Environment lạ → từ chối.
5. gRPC giới hạn mỗi kết nối tối đa `MAX_CONCURRENT_REQUESTS` stream (stream vượt mức chờ ở client thay vì
   nhận lỗi), handshake 10s, keepalive và tuổi kết nối 30 phút để cân bằng lại giữa replica.
   Lấy slot xử lý (hết slot → 503), giải mã (giới hạn cả body nén và giải nén), chuẩn hóa, che key
   nhạy cảm và pattern secret trong văn bản tự do. Record sai → đếm `rejected`, vẫn xử lý phần còn lại.
6. Đưa vào pipeline theo kiểu được cả hoặc không: giữ slot và byte cho cả request một lần, rồi mới đưa event
   vào queue. Không đủ chỗ trong `ENQUEUE_TIMEOUT` → 503 + `Retry-After`, không event nào của request được ghi,
   nên exporter gửi lại không làm PG ghi trùng phần đã nhận. Chỉ request lớn hơn cả queue mới được nhận theo phần.
   Sức chứa = `QUEUE_SIZE` + `WORKERS` × `BATCH_SIZE`; slot giữ tới khi event được ACK.
7. Worker gom batch theo scope, dùng pipeline cũ. `application/logs.Persist`
   ghi raw log bằng HTTP/gzip JSONEachRow (`async_insert=1`, `wait_for_async_insert=1`),
   rồi gọi `StoreEvents` cũ cho trace, `StoreLogIncidents` cho lỗi log. Hàm sau chỉ giữ
   envelope lỗi (attributes rỗng, không raw body/resource/scope attributes), gọi lại
   `RecordOccurrences` và outbox trong cùng transaction PostgreSQL. Log thường không ghi PG.
   Span vẫn đánh dấu phút dirty cùng transaction. Metrics đi pipeline PostgreSQL riêng như cũ.
8. Tất cả bước thành công mới ACK; không có transaction xuyên hai database. Bước sau lỗi
   có thể để lại dữ liệu đã ghi ở bước trước; exporter retry với cùng ID. ClickHouse dùng
   ReplacingMergeTree và đọc `FINAL` để khử trùng, PostgreSQL giữ idempotency hiện có.
   Storage tạm lỗi → 503/Unavailable. Event có nội dung không thể lưu → chia đôi batch
   tách record hỏng, phần còn lại commit và trả partial success.
   Mỗi batch event thử ghi tối đa 3 lần, chỉ backoff giữa hai lần thử; hủy trong lúc chờ
   dừng ngay, không gọi storage thêm. Metric retry chỉ đếm lần thử lại thực sự bắt đầu.

API ListLogs/GetLog và số đếm log trong TelemetryQuality đọc trực tiếp từ ClickHouse,
giữ project/environment, filter, cursor và correlation trace/span. Không trộn hai kho khi phân trang.
PostgreSQL không lưu log và không đọc dự phòng: ClickHouse lỗi thì ingest log trả 503, đọc log trả lỗi.
`Database.StoreEvents` từ chối event log như lỗi storage (không chia batch rồi bỏ như dữ liệu hỏng).
Bản cài cũ còn log trong PG phải chép sang ClickHouse bằng `migrate clickhouse backfill` của bản trước
rồi mới nâng cấp; migration `000002` xóa bảng `logs`.

## 6. Vòng đời incident [6]

- Mỗi lỗi trực tiếp tạo occurrence; **fingerprint** = service + operation + error type + HTTP status +
  message đã chuẩn hóa (UUID, thời điểm, email, IP, số bị thay placeholder). Cùng fingerprint → cùng incident.
- Trạng thái: `OPEN`, `INVESTIGATING`, `RESOLVED`, `IGNORED`. Người dùng đổi trạng thái (scope `write`),
  mỗi lần ghi lịch sử kèm người thực hiện.
- `RESOLVED` mà có occurrence mới hơn lúc resolve (trong cửa sổ dedup 1 giờ) → tự mở lại, người thực hiện
  là `system`, phát webhook `incident.opened` với `reopened: true`. `IGNORED` không tự mở lại.
- Console mở trang incident bằng hai truy vấn: thông tin incident và analysis. Header giữ trạng thái,
  severity, service, tổng occurrence và first/last seen. Investigation, Occurrences, Linked telemetry và
  Status history là các tab hỗ trợ bàn phím, giữ tab trên URL. Ba tab dữ liệu chỉ gọi API khi được mở;
  cursor tải 25 bản ghi/trang, chỉ dựng trang đang xem và giữ tối đa 20 trang đã tải trong cache.
  Analysis hiển thị tối đa 3 giả thuyết, 8 evidence và 8 quan hệ mỗi trang. Score là heuristic, không phải
  xác suất hay kết luận root cause. Chọn giả thuyết lọc evidence theo ID; số liệu gốc, series, ID và toàn bộ
  trace link vẫn mở được trong chi tiết. Đổi trạng thái thành công cập nhật header và làm mới lịch sử;
  thất bại giữ trạng thái cũ và các tab vẫn dùng được.

## 7. Phát hiện bất thường và RCA [7]

Mỗi phút, với từng cặp project/environment: worker pool chạy song song có giới hạn (tối đa 4 goroutines)
so cửa sổ 5 phút gần nhất với baseline 30 phút trước đó. SQL lấy tín hiệu, policy thuần ở domain quyết định
lệch (latency cần ≥ 10 call ở cả hai cửa sổ; throughput cần baseline ≥ 30 call). Có lệch → tạo/cập nhật
incident kèm evidence. Khi xem incident, RCA lấy evidence và quan hệ phụ thuộc rồi xếp hạng giả thuyết
theo độ tin cậy. Đây là giả thuyết có bằng chứng, không phải kết luận.

Trước khi detection chạy, worker cũ tổng hợp tối đa 64 phút đang dirty của mỗi scope vào
`span_minute_rollups`, với deadline riêng 5 giây. Worker khóa các phút bằng `FOR UPDATE SKIP LOCKED`,
rồi đọc snapshot mới để thay thế toàn bộ summary và đánh dấu clean trong cùng transaction.
Span đến muộn hoặc retry đánh dấu phút dirty lại; không cộng dồn nên không đếm trùng.
Refresh lỗi thì detection vẫn thử đường đọc gốc trong ngân sách còn lại.

Các hàm `RequestPerformance`, `DetectServiceEvidence`, `DetectDependencyEvidence` và policy domain
giữ nguyên contract. Detection ghép count và tổng duration của các phút clean, chỉ đọc span gốc cho
phút dirty, chưa được tổng hợp hoặc cắt ngang ranh giới cửa sổ. Performance đọc các mảng timestamp,
duration và trạng thái lỗi đã chuẩn bị từ summary; vẫn tính percentile trên từng request, không lấy
trung bình p95/p99 giữa các phút. Vì vậy cửa sổ bất kỳ, bucket biểu đồ và mẫu trace evidence giữ nguyên.
OTLP metrics/counter reset, topology và trace/log explorer tiếp tục dùng các hàm hiện có.

Topology giới hạn parent trong cùng project/environment và từ một giờ trước cửa sổ con.
SQL dùng LATERAL và OFFSET 0 để tra parent theo trace/span ID; danh sách trace cũng tra span theo
từng trace ID. Cách này giữ lookup dùng index khi project mới chưa có trong thống kê, tránh quét
lặp mọi cặp span. Test tích hợp ANALYZE partition con của scope cũ rồi thêm 16.000 spans vào scope
mới, kiểm tra cả topology và danh sách trace trong ngân sách 5 giây mỗi truy vấn.

Phần rollup trong baseline chỉ lập danh sách phút cần xử lý; worker xử lý dần, không dựng
mảng latency trong migration. Khi worker chưa xử lý hết, query vẫn trả dữ liệu từ spans trên cùng
snapshot với summary, nên không cần chờ backfill và không thêm cờ cấu hình hay tiến trình.

## 8. Ingest token [8]

Owner tạo token (tên duy nhất trong project, mặc định hết hạn sau 1 năm, tối đa 3 năm). JWT chỉ hiện **một lần**;
DB chỉ lưu dòng metadata. Token được ký bằng khóa riêng biệt `INGEST_JWT_SECRET`. Production bắt buộc khóa này và phải khác `JWT_SECRET`;
ngoài production, thiếu khóa thì dùng tạm `JWT_SECRET` kèm cảnh báo.
Dùng token: bộ thu tra dòng đó qua cache với TTL cấu hình; khi chủ project thu hồi token qua API,
cache local của tiến trình xử lý yêu cầu và entry shared cache được xóa. Replica khác có thể còn
giữ entry local tới khi hết TTL; chưa cam kết thu hồi tức thì trên toàn cụm. Tách riêng khóa ký
giúp xoay khóa tài khoản người dùng mà không gián đoạn OTLP ingestion của máy móc.

## 9. Webhook và outbox [9]

Incident mở/mở lại → xếp delivery vào outbox **trong cùng transaction** ingest. Worker nhận việc bằng
`FOR UPDATE SKIP LOCKED` (lease 6 phút nên crash được nhận lại), kiểm URL (HTTPS, chặn dải private/metadata,
không theo redirect), ký HMAC và gửi. Thất bại → thử lại theo chính sách tới `failed`. Nhận ít nhất một lần.
Mỗi lượt chỉ nhận tối đa `NOTIFICATIONS_WORKERS` delivery và chờ cả batch gửi/ghi kết quả xong
trước lượt poll tiếp theo, nên endpoint chậm không làm các lượt poll cộng dồn worker hoặc lease.

## 10. Partition và retention [10]

Bảng telemetry PostgreSQL chia partition theo ngày UTC. Khởi động tạo `DATABASE_PARTITION_WINDOW_DAYS` (10) ngày từ hôm qua,
bảo trì mỗi `DATABASE_PARTITION_CHECK_INTERVAL` (1 giờ). Retention
chạy theo chu kỳ, mỗi nhóm một `max_age` (`0s` = giữ mãi): telemetry 7 ngày (drop partition cũ), incident đã
đóng 90 ngày (kéo theo evidence, occurrence, lịch sử), audit 180 ngày, webhook delivery 30 ngày. Incident đang
mở và delivery đang xử lý không bao giờ bị xóa.
Retention dọn event/span/log và xóa bucket summary trước cutoff (kéo theo rollup) trong cùng
transaction; lỗi bất kỳ thì rollback cả telemetry và summary. Phần phút còn
span sống sót được đọc từ dữ liệu gốc cho tới khi có ingest mới đánh dấu cần tổng hợp lại.

ClickHouse raw log hỗ trợ cấu hình phân vùng thời gian qua `TRACEPILOT_CLICKHOUSE_LOG_PARTITION`: ngày (`toYYYYMMDD`), tháng (`toYYYYMM`), năm (`toYear`), hoặc tuần (`toStartOfWeek`, mặc định).
`ReplacingMergeTree` đọc `FINAL`, khóa scope/service + timestamp millisecond + event ID để
exporter retry cùng event ID được khử trùng. Hash ID bao gồm timestamp đầy đủ tới nanosecond, nên hai log cùng nội dung trong cùng millisecond vẫn khác nhau; timestamp/cursor gốc giữ nanosecond. TTL lấy `RETENTION_MAX_AGE`, API luôn lọc tuổi log. `ttl_only_drop_parts=1`
chỉ xóa part hết hạn toàn bộ nên vật lý có thể lưu thêm partition period cộng độ trễ merge.
Đổi TTL phải chạy `migrate clickhouse up`, `migrate all` hoặc bật auto migration CH; readiness từ chối schema không hợp lệ.
Cấu hình partition chọn khóa cho bảng mới; đổi khóa của bảng hiện có cần migration bảng/dữ liệu riêng.

Log explorer và Live Tail lọc cùng nhóm severity TRACE/DEBUG/INFO/WARN/ERROR/FATAL. Số OTLP 1–24
được ưu tiên (mỗi nhóm bốn mức); số unspecified dùng text không phân biệt hoa thường, nhận cả
WARNING/INFORMATION và hậu tố 1–4. Text trống mặc định INFO; text riêng vẫn có thể lọc không phân biệt
hoa thường. Kho dữ liệu và response giữ nguyên text exporter, SSE gửi thêm `severity_number`.

## 11. Telemetry streaming thời gian thực (SSE) và tối ưu Waterfall

- **Live Streaming qua SSE:** Tuyến `GET /api/v1/live/stream` (scope `read`) mở kênh Server-Sent Events (SSE)
  truyền trực tiếp log mới và sự cố phát sinh sau khi mọi bước persist của batch thành công. Stream chạy qua bộ nhớ
  `application/live.Hub` độc lập với HTTP và storage, được callback của pipeline gọi sau persist;
  HTTP adapter đăng ký nhận event, serialize JSON và đóng kết nối khi hub dừng. Hub không chiếm slot
  truy vấn bounded concurrency (`admitQuery`), có giới hạn 256 kết nối đồng thời
  và dùng deadline ghi 5 giây cho từng frame để client chậm không giữ tài nguyên vô hạn.
  Stream kết thúc khi JWT hết hạn; mỗi 15 giây kiểm tra tài khoản và membership trực tiếp ở PostgreSQL,
  không dùng role cache, với deadline kiểm tra 2 giây. Lỗi kiểm tra hoặc quyền bị thu hồi đóng stream.
  Console dùng chung API client cho JSON và SSE (JWT thật, API origin, project/environment, refresh token).
  Lỗi quyền 4xx được hiển thị; lỗi tạm kết nối lại với backoff tối đa 15 giây. Live Tail giữ tối đa
  100 event mới, áp dụng service/severity/body/trace/span filter và khử trùng với kết quả lịch sử.
- **Tối ưu hóa Trace Waterfall:** Cây span được ảo hóa hiển thị (`useVirtualList` zero-dependency); với các trace
  phức tạp có hàng nghìn span, trình duyệt chỉ render các phần tử trong khung cuộn (viewport) kèm vùng đệm, giữ
  cho DOM nhẹ ngay từ lần render đầu. Dòng có chiều cao cố định; inspector chi tiết nằm ngoài danh sách
  nên attributes hoặc stack trace dài không chồng lên span khác. Console tải tối đa 5.000 span/trace
  và thông báo rõ khi bị cắt. Đóng inspector trả focus về nút đã mở nó.
- **Điều tra trace:** Header hiển thị operation, trạng thái và bốn chỉ số; dải request flow tóm tắt self-time
  theo service, service map mở theo yêu cầu. Tìm operation/service/span ID và lọc lỗi trên toàn bộ span đã tải,
  kể cả các nhánh đang thu gọn. Chọn span mở inspector bên phải trên desktop, xếp dưới trên màn hình nhỏ;
  Attributes/Resource/Events/Raw chuyển bằng click hoặc phím mũi tên, Home/End. Log liên quan nằm dưới
  timeline; mỗi dòng mở nội dung đầy đủ và sao chép khi cần. Bộ lọc log cục bộ chỉ tìm các log đã tải,
  còn Log explorer truy vấn server với trace/span và khoảng thời gian được giữ lại.
- **Explorer và Overview:** bảng cursor tải thêm theo thao tác người dùng, tối đa 20 trang cho mỗi truy vấn;
  chạm giới hạn thì yêu cầu thu hẹp bộ lọc. Timestamp hiển thị millisecond và timezone local.
  Overview ghi rõ count thuộc trang đã tải; tín hiệu lỗi tổng lấy từ performance trong cùng cửa sổ.
  Trang incident chưa đầy đủ hoặc traffic chưa có kết quả không được suy diễn thành trạng thái toàn hệ thống bình thường.

## 12. Giới hạn đã biết

- Rate limit tính theo từng tiến trình; nhiều replica thì đặt thêm ở reverse proxy.
- Live Tail là best effort từ bộ nhớ của tiến trình đang phục vụ kết nối; chưa fan-out giữa replica.
- Ngân sách byte của queue là ước lượng, không phải giới hạn RSS (gRPC giải mã trước khi vào hàng đợi;
  giới hạn stream chỉ tính theo từng kết nối).
- Chưa có SSO/MFA, đặt lại mật khẩu qua email, xóa project/environment.
- Số đo tải và giới hạn triển khai nằm trong README (Measured capacity). `ListTraces` tổng hợp các trace khớp bộ lọc
  trước khi lấy `limit`, nên chi phí tăng theo số span. Request không có `from`
  (và không lọc `traceId`) chỉ đọc 1 giờ trước `to`, con trỏ trang hoặc hiện tại; console kiểm tra kết nối bằng
  `GET /projects/{id}` thay vì liệt kê trace. Lần đo 2026-11-16 đã bỏ timeout 504 do lookup lặp span,
  nhưng 1.000 bản ghi + 5 truy vấn/giây vẫn bão hòa PostgreSQL 2 vCPU và trả 503 do backpressure.
  Cùng code và workload với PostgreSQL 4 vCPU không lỗi trong 60 giây, đọc p95 689ms; đây là số đo
  ngắn để chọn cấu hình thử nghiệm, chưa chứng minh tải dài hạn hay cửa sổ truy vấn đầy một giờ.
- ClickHouse sập làm readiness trả 503 trên mọi replica (đăng nhập vẫn chạy); sau khi CH lên lại cần khoảng 30s.
- Test tích hợp PostgreSQL cần database dùng một lần (`TRACEPILOT_INTEGRATION_TEST=1`),
  ClickHouse dùng instance dùng một lần (`TRACEPILOT_CLICKHOUSE_INTEGRATION_TEST=1`, `TRACEPILOT_CLICKHOUSE_TEST_URL`).
- Bộ migration đã tách lại cho database mới trước khi phát hành; không tự nâng cấp lịch sử cũ
  hay tự đặt lại version trên database thật.
