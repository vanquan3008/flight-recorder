# Luồng hoạt động của TracePilot

Mô tả các luồng chính của hệ thống. Sơ đồ tương ứng ở [diagrams.html](diagrams.html)
(mở bằng trình duyệt, cần mạng để tải thư viện vẽ). Số trong ngoặc `[n]` là số sơ đồ.

Một ứng dụng Go (API quản trị + bộ thu OTLP + worker nền), PostgreSQL riêng, và
console React là ứng dụng riêng.

## 1. Tổng quan [1]

- Ứng dụng của bạn gửi OTLP (gRPC `:4317`, HTTP `:4318`) kèm **ingest token**.
- Bộ thu xác thực, chuẩn hóa, đưa vào **pipeline** có giới hạn; pipeline ghi batch
  vào PostgreSQL. Chỉ khi transaction **commit** mới trả ACK cho ứng dụng.
- Cùng transaction ghi event/span/log còn tạo/cập nhật **incident** và xếp **webhook**
  vào outbox.
- Worker nền: phát hiện bất thường mỗi phút, gửi webhook, tạo partition, dọn dữ liệu hết hạn.
- Người dùng vào **console**, console gọi REST `/api/v1` (`:8080`) bằng access token.

## 2. Khởi động và tắt [2]

Khởi động theo thứ tự, bước nào lỗi thì thoát với mã 1:

1. Đọc cấu hình (mặc định → `backend/.env` → biến môi trường). Sai cấu hình → mã 2.
2. Không có `TRACEPILOT_AUTH_JWT_SECRET` → sinh khóa ngẫu nhiên và cảnh báo (production
   thì cấu hình sai ngay từ bước 1).
3. Kiểm tra schema đúng version (`CurrentVersion = 2`, không dirty). Server **không tự migrate**:
   chạy `tracepilot migrate up` riêng.
4. Mở pool, tạo partition theo ngày, tạo project/environment mặc định.
5. Dựng replay (nếu bật), metrics, hai pipeline (events, metrics), bộ thu OTLP, API.
6. Chạy song song: API, OTLP, detection, bảo trì partition, webhook (nếu bật), retention
   (nếu có max_age). Một thành phần lỗi hoặc tự dừng thì cả tiến trình dừng.

Tắt (SIGINT/SIGTERM hoặc một thành phần lỗi): listener ngừng nhận và chờ request, rồi pipeline
events và pipeline metrics drain lần lượt trong `shutdown_timeout` (15s). Đặt grace của container
≥ 35s. Exporter gửi lại phần chưa ACK.

## 3. Đăng ký, đăng nhập, refresh [3]

- **Đăng ký:** tài khoản đầu tiên luôn được phép, thành **super admin** và owner của project mặc
  định. Các tài khoản sau chỉ đăng ký được khi `ALLOW_REGISTRATION=true`. Mật khẩu 12–256 ký tự.
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
5. Route theo project: lấy project từ path hoặc `X-Project-ID` (ID hoặc slug). Project không có và
   project không được phép đều trả cùng 403 để không dò được tên.
6. Vai trò: super admin = owner ở mọi project; người khác lấy từ `project_members`.
   `user` → `read, write`; `owner` → thêm `replay, admin`. Thiếu scope → 403.
7. Resolve environment (`X-Environment`, mặc định là cái đầu tiên); sai → 400.
8. Xin một trong 4 slot truy vấn (chờ 250 ms, hết → 503), chạy dưới deadline 20 s (quá → 504).

## 5. Ingest OTLP [5]

1. Request đến HTTP hoặc gRPC; lấy `Authorization: Bearer <ingest token>` và `X-Environment` (gRPC:
   metadata `x-environment`).
2. Kiểm sổ chặn: client sai token quá `AUTH_FAILURE_LIMIT` lần/cửa sổ → 429, không truy vấn DB.
3. Xác thực: chữ ký, issuer/audience, loại `ingest`, hạn; sau đó tra `ingest_tokens` (chưa thu hồi,
   chưa hết hạn, đúng project). Token sai → 401; DB lỗi → 503 để exporter retry.
4. Phân giải scope (project + environment) qua `scopeCatalog` (cache 30 s). Environment lạ → từ chối.
5. Lấy slot xử lý (hết slot → 503), giải mã (giới hạn cả body nén và giải nén), chuẩn hóa, che key
   nhạy cảm và pattern secret trong văn bản tự do. Record sai → đếm `rejected`, vẫn xử lý phần còn lại.
6. Đưa vào pipeline: giới hạn số event và byte; đầy → 503 + `Retry-After`.
7. Worker gom batch, gom theo scope, ghi **một transaction**: events + span/log + incident/occurrence/
   evidence + outbox webhook. Ghi idempotent theo ID tất định.
8. Commit xong mới ACK. Storage tạm lỗi → 503/Unavailable (exporter gửi lại, không trùng bản ghi).
   PostgreSQL từ chối nội dung của một event → chia đôi batch tới khi tách được event hỏng, phần còn
   lại vẫn commit, trả partial success.

## 6. Vòng đời incident [6]

- Mỗi lỗi trực tiếp tạo occurrence; **fingerprint** = service + operation + error type + HTTP status +
  message đã chuẩn hóa (UUID, thời điểm, email, IP, số bị thay placeholder). Cùng fingerprint → cùng incident.
- Trạng thái: `OPEN`, `INVESTIGATING`, `RESOLVED`, `IGNORED`. Người dùng đổi trạng thái (scope `write`),
  mỗi lần ghi lịch sử kèm người thực hiện.
- `RESOLVED` mà có occurrence mới hơn lúc resolve (trong cửa sổ dedup 1 giờ) → tự mở lại, người thực hiện
  là `system`, phát webhook `incident.opened` với `reopened: true`. `IGNORED` không tự mở lại.

## 7. Phát hiện bất thường và RCA [7]

Mỗi phút, với từng cặp project/environment: so cửa sổ 5 phút gần nhất với baseline 30 phút trước đó.
SQL lấy tín hiệu, policy thuần ở domain quyết định lệch (latency cần ≥ 10 call ở cả hai cửa sổ; throughput
cần baseline ≥ 30 call). Có lệch → tạo/cập nhật incident kèm evidence. Khi xem incident, RCA lấy evidence
và quan hệ phụ thuộc rồi xếp hạng giả thuyết theo độ tin cậy. Đây là giả thuyết có bằng chứng, không phải kết luận.

## 8. Ingest token [8]

Owner tạo token (tên duy nhất trong project, mặc định hết hạn sau 1 năm, tối đa 3 năm). JWT chỉ hiện **một lần**;
DB chỉ lưu dòng metadata. Dùng token: bộ thu tra dòng đó mỗi request nên **thu hồi có hiệu lực ngay**. Đổi
`JWT_SECRET` thu hồi mọi token và phiên.

## 9. Webhook và outbox [9]

Incident mở/mở lại → xếp delivery vào outbox **trong cùng transaction** ingest. Worker nhận việc bằng
`FOR UPDATE SKIP LOCKED` (lease 6 phút nên crash được nhận lại), kiểm URL (HTTPS, chặn dải private/metadata,
không theo redirect), ký HMAC và gửi. Thất bại → thử lại theo chính sách tới `failed`. Nhận ít nhất một lần.

## 10. Partition và retention [10]

Bảng telemetry chia partition theo ngày UTC. Khởi động tạo 10 ngày từ hôm qua, bảo trì mỗi giờ. Retention
chạy theo chu kỳ, mỗi nhóm một `max_age` (`0s` = giữ mãi): telemetry 7 ngày (drop partition cũ), incident đã
đóng 90 ngày (kéo theo evidence, occurrence, lịch sử), audit 180 ngày, webhook delivery 30 ngày. Incident đang
mở và delivery đang xử lý không bao giờ bị xóa.

## 11. Giới hạn đã biết

- Rate limit tính theo từng tiến trình; nhiều replica thì đặt thêm ở reverse proxy.
- Ngân sách byte của queue là ước lượng, không phải giới hạn RSS (gRPC giải mã trước khi vào hàng đợi).
- Mỗi request OTLP tốn một truy vấn kiểm ingest token.
- Chưa có SSO/MFA, đặt lại mật khẩu qua email, xóa project/environment.
- Chưa đo tải/SLO; test tích hợp PostgreSQL cần database dùng một lần (`TRACEPILOT_INTEGRATION_TEST=1`).
- DB tạo bằng lịch sử migration cũ (version 1–5) không nâng cấp tại chỗ; version 2 cũ **trùng số** với
  `CurrentVersion` hiện tại nên server không tự phát hiện — đặt lại `schema_migrations` về 1 rồi `migrate up`,
  hoặc dùng database mới.
