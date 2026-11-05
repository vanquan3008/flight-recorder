# Luồng hoạt động của TracePilot

Mô tả các luồng chính của hệ thống. Sơ đồ tương ứng ở [diagrams.html](diagrams.html)
(mở bằng trình duyệt, cần mạng để tải thư viện vẽ). Số trong ngoặc `[n]` là số sơ đồ.

Một ứng dụng Go (API quản trị + bộ thu OTLP + worker nền), PostgreSQL riêng,
ClickHouse tùy chọn cho raw log và console React riêng.

## 1. Tổng quan [1]

- Ứng dụng của bạn gửi OTLP (gRPC `:4317`, HTTP `:4318`) kèm **ingest token**.
- Bộ thu xác thực, chuẩn hóa, đưa vào **pipeline** có giới hạn; pipeline ghi batch
  theo scope. Trace/metric vào PostgreSQL, raw log vào ClickHouse khi cấu hình URL;
  URL trống giữ cách lưu PostgreSQL cũ.
- Log thường chỉ ghi ClickHouse; log gây incident giữ envelope lỗi gọn trong PostgreSQL.
  Incident/evidence/occurrence và outbox vẫn commit cùng transaction PostgreSQL.
  ACK chờ ClickHouse flush và mọi transaction PostgreSQL cần thiết thành công.
- Worker nền: phát hiện bất thường mỗi phút, gửi webhook, tạo partition, dọn dữ liệu hết hạn.
- Người dùng vào **console**, console gọi REST `/api/v1` (`:8080`) bằng access token.

## 2. Khởi động và tắt [2]

Khởi động theo thứ tự, bước nào lỗi thì thoát với mã 1:

1. Đọc cấu hình (mặc định → `backend/.env` → biến môi trường). Sai cấu hình → mã 2.
2. Không có `TRACEPILOT_AUTH_JWT_SECRET` → sinh khóa ngẫu nhiên và cảnh báo (production
   thì cấu hình sai ngay từ bước 1).
3. Kiểm tra schema đúng version (`CurrentVersion = 3`, không dirty). Server **không tự migrate**:
   chạy `tracepilot migrate up` riêng.
4. Mở pool; nếu bật ClickHouse, kiểm tra schema tuần UTC đã được migrate riêng
   (`migrate clickhouse up`). Tạo partition PostgreSQL ngày và project/environment mặc định.
   Readiness kiểm tra cả hai kho; không tự fallback khi ClickHouse hỏng.
5. Dựng replay (nếu bật), metrics, hai pipeline (events, metrics), bộ thu OTLP, API.
6. Chạy song song: API, OTLP (thành phần cốt lõi; lỗi hoặc dừng thì tiến trình dừng),
   cùng detection, bảo trì partition, webhook (nếu bật), retention (nếu có max_age)
   được giám sát phục hồi tự động (restart with backoff) để đảm bảo tính sẵn sàng cao.

Tắt (SIGINT/SIGTERM hoặc một thành phần lỗi): listener ngừng nhận và chờ request, rồi pipeline
events và pipeline metrics drain lần lượt trong `shutdown_timeout` (15s). Đặt grace của container
≥ 35s. Exporter gửi lại phần chưa ACK.

## 3. Đăng ký, đăng nhập, refresh [3]

- **Đăng ký:** super admin development được tạo qua seed (`tracepilot seed` với `admin@tracepilot.local` / `admin-password-123`). Các tài khoản khác đăng ký qua form/API khi `ALLOW_REGISTRATION=true` và khởi tạo ở quyền tài khoản thông thường. Sau đó super admin sẽ cấp quyền sử dụng: cấp cờ super admin (`platform_admin`) trong Settings → Accounts hoặc thêm vào project trong Settings → Members. Mật khẩu 12–256 ký tự. Khi `ALLOW_REGISTRATION=false`, đăng ký tự do bị đóng hoàn toàn.
- **Đăng nhập:** giới hạn theo IP và 10 lần/email/cửa sổ. Email lạ hay sai mật khẩu đều tốn cùng
  thời gian và cùng một thông báo. Thành công → server tạo phiên refresh, đặt cookie
  `tp_refresh` (`HttpOnly`, `SameSite=Strict`, path `/api/v1/auth`) và trả **access token** (JWT HS256,
  15 phút) cùng danh sách project.
- **Access token** chỉ nằm trong bộ nhớ trình duyệt. Hết hạn → API trả `TOKEN_EXPIRED` → console
  gọi `/auth/refresh` (kèm header `X-TracePilot-Refresh`) rồi gửi lại request. Mỗi lần refresh đổi
  token mới; dùng lại token cũ bị từ chối. Phiên hết hạn sau 7 ngày không dùng hoặc 30 ngày tuyệt đối.
- **Đổi mật khẩu / khóa tài khoản:** đặt `tokens_valid_after`, xóa các phiên khác, nên access token
  cấp trước đó mất hiệu lực ngay. Tài khoản do admin tạo có mật khẩu tạm: chỉ gọi được
  `/auth/me` và `/auth/password` cho tới khi đổi.

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
5. Lấy slot xử lý (hết slot → 503), giải mã (giới hạn cả body nén và giải nén), chuẩn hóa, che key
   nhạy cảm và pattern secret trong văn bản tự do. Record sai → đếm `rejected`, vẫn xử lý phần còn lại.
6. Đưa vào pipeline: giới hạn số event và byte; đầy → 503 + `Retry-After`.
7. Worker gom batch theo scope, dùng pipeline cũ. Khi bật ClickHouse, `application/logs.Persist`
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

API ListLogs/GetLog và số đếm log trong TelemetryQuality đọc cùng kho đang được cấu hình,
giữ project/environment, filter, cursor và correlation trace/span cũ. Không trộn hai kho khi phân trang.
Chuyển dữ liệu cũ: dừng ingest/drain → `migrate clickhouse up` → `migrate clickhouse backfill`
→ khởi động mọi replica với cùng cấu hình → mở ingest. Backfill có thể chạy lại, không xóa log PG.

## 6. Vòng đời incident [6]

- Mỗi lỗi trực tiếp tạo occurrence; **fingerprint** = service + operation + error type + HTTP status +
  message đã chuẩn hóa (UUID, thời điểm, email, IP, số bị thay placeholder). Cùng fingerprint → cùng incident.
- Trạng thái: `OPEN`, `INVESTIGATING`, `RESOLVED`, `IGNORED`. Người dùng đổi trạng thái (scope `write`),
  mỗi lần ghi lịch sử kèm người thực hiện.
- `RESOLVED` mà có occurrence mới hơn lúc resolve (trong cửa sổ dedup 1 giờ) → tự mở lại, người thực hiện
  là `system`, phát webhook `incident.opened` với `reopened: true`. `IGNORED` không tự mở lại.

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

Migration `000003_span_rollups` chỉ lập danh sách phút của dữ liệu cũ; worker xử lý dần, không dựng
mảng latency trong migration. Khi worker chưa xử lý hết, query vẫn trả dữ liệu từ spans trên cùng
snapshot với summary, nên không cần chờ backfill và không thêm cờ cấu hình hay tiến trình.

## 8. Ingest token [8]

Owner tạo token (tên duy nhất trong project, mặc định hết hạn sau 1 năm, tối đa 3 năm). JWT chỉ hiện **một lần**;
DB chỉ lưu dòng metadata. Token được ký bằng khóa riêng biệt `INGEST_JWT_SECRET` (hoặc fallback `JWT_SECRET` nếu chưa cấu hình).
Dùng token: bộ thu tra dòng đó qua cache bộ nhớ (30s); khi chủ project thu hồi token qua API, cache xóa ngay lập tức
nên **thu hồi có hiệu lực ngay**. Tách riêng khóa ký giúp xoay khóa tài khoản người dùng mà không gián đoạn OTLP ingestion của máy móc.

## 9. Webhook và outbox [9]

Incident mở/mở lại → xếp delivery vào outbox **trong cùng transaction** ingest. Worker nhận việc bằng
`FOR UPDATE SKIP LOCKED` (lease 6 phút nên crash được nhận lại), kiểm URL (HTTPS, chặn dải private/metadata,
không theo redirect), ký HMAC và gửi. Thất bại → thử lại theo chính sách tới `failed`. Nhận ít nhất một lần.

## 10. Partition và retention [10]

Bảng telemetry PostgreSQL chia partition theo ngày UTC. Khởi động tạo 10 ngày từ hôm qua, bảo trì mỗi giờ. Retention
chạy theo chu kỳ, mỗi nhóm một `max_age` (`0s` = giữ mãi): telemetry 7 ngày (drop partition cũ), incident đã
đóng 90 ngày (kéo theo evidence, occurrence, lịch sử), audit 180 ngày, webhook delivery 30 ngày. Incident đang
mở và delivery đang xử lý không bao giờ bị xóa.
Retention dọn event/span/log và xóa bucket summary trước cutoff (kéo theo rollup) trong cùng
transaction; lỗi bất kỳ thì rollback cả telemetry và summary. Phần phút còn
span sống sót được đọc từ dữ liệu gốc cho tới khi có ingest mới đánh dấu cần tổng hợp lại.

ClickHouse raw log chia tuần từ thứ Hai UTC bằng `toStartOfWeek(occurred_at, 1, 'UTC')`.
`ReplacingMergeTree` đọc `FINAL`, khóa scope/service + timestamp millisecond + event ID để
log backfill từ PostgreSQL microsecond vẫn khử trùng với retry nanosecond; timestamp/cursor
gốc giữ nanosecond. TTL lấy `RETENTION_MAX_AGE`, API luôn lọc tuổi log. `ttl_only_drop_parts=1`
chỉ xóa part hết hạn toàn bộ nên vật lý có thể lưu thêm gần một tuần cộng độ trễ merge.
Đổi TTL phải chạy `migrate clickhouse up`; startup/readiness từ chối schema hoặc TTL lệch.

## 11. Telemetry streaming thời gian thực (SSE) và tối ưu Waterfall

- **Live Streaming qua SSE:** Tuyến `GET /api/v1/live/stream` (scope `read`) mở kênh Server-Sent Events (SSE)
  truyền trực tiếp log mới và sự cố phát sinh sau khi mọi bước persist của batch thành công. Stream chạy qua bộ nhớ
  `StreamHub` độc lập, không chiếm slot truy vấn bounded concurrency (`admitQuery`), có giới hạn 256 kết nối đồng thời
  và bỏ write deadline riêng cho stream để không bị ngắt sau 30 giây.
  Console cung cấp chế độ **Live Tail** trên trang Logs để theo dõi dòng log trực tiếp theo thời gian thực.
- **Tối ưu hóa Trace Waterfall:** Cây span được ảo hóa hiển thị (`useVirtualList` zero-dependency); với các trace
  phức tạp có hàng nghìn span, trình duyệt chỉ render các phần tử trong khung cuộn (viewport) kèm vùng đệm, giữ
  cho DOM nhẹ và mượt mà mà không làm đứt đoạn trục thời gian.

## 12. Giới hạn đã biết

- Rate limit tính theo từng tiến trình; nhiều replica thì đặt thêm ở reverse proxy.
- Ngân sách byte của queue là ước lượng, không phải giới hạn RSS (gRPC giải mã trước khi vào hàng đợi).
- Chưa có SSO/MFA, đặt lại mật khẩu qua email, xóa project/environment.
- Chưa đo tải/SLO production; test tích hợp PostgreSQL cần database dùng một lần (`TRACEPILOT_INTEGRATION_TEST=1`),
  ClickHouse dùng instance dùng một lần (`TRACEPILOT_CLICKHOUSE_INTEGRATION_TEST=1`, `TRACEPILOT_CLICKHOUSE_TEST_URL`).
- DB tạo bằng lịch sử migration cũ trước khi gộp schema (version 1–5) cần kiểm tra schema thực tế trước
  khi nâng cấp; số version có thể trùng với lịch sử hiện tại. Không tự đặt lại version trên database thật.
