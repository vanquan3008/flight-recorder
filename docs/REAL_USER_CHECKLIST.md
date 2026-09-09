# Checklist đưa TracePilot tới người dùng thật

Mục tiêu của checklist này không phải “ship nhiều feature”. Mục tiêu là chứng minh
một người ngoài đội dự án có vấn đề thật, tự cài được TracePilot, nhận được giá trị
và tiếp tục dùng.

Chỉ đánh dấu hoàn tất khi có link tới bằng chứng (issue, recording, dashboard,
release hoặc interview note). Mỗi mục đang làm phải có owner và hạn hoàn thành.

## Định nghĩa và thước đo

- [ ] Chốt định nghĩa **real user**: không phải maintainer; chạy với ứng dụng/dữ liệu
      của họ; ingest ít nhất 7 ngày; quay lại console từ 2 ngày khác nhau.
- [ ] Chốt primary ICP bằng một câu, ví dụ: “team backend 2-10 người cần self-host
      trace + log để điều tra lỗi mà không vận hành một observability stack lớn”.
- [ ] Chốt một primary job: “từ alert/lỗi đến trace và log nguyên nhân trong dưới
      5 phút”.
- [ ] Chọn north-star cho beta: số project bên ngoài có ingest và một phiên điều tra
      thành công mỗi tuần.
- [ ] Ghi công thức cho bốn metric: setup success, time-to-first-trace, activation
      trong 24 giờ và retention tuần 4.
- [ ] Tạo bảng bằng chứng tối thiểu: `user/project`, `nguồn`, `ngày cài`, `first
      signal`, `activated`, `week-1`, `week-4`, `blocker`, `owner`.
- [ ] Nếu thu product telemetry, dùng opt-in, công bố chính xác dữ liệu gửi đi và
      cho phép tắt hoàn toàn; nếu chưa cần, theo dõi thủ công cho 5-10 beta đầu.

## Gate 1 — Problem/ICP có thật

- [ ] Phỏng vấn 10 người đúng ICP; không demo trước khi hỏi workflow hiện tại,
      tần suất sự cố, công cụ, chi phí và lần đau gần nhất.
- [ ] Có ít nhất 6/10 người gặp cùng một job/problem hàng tháng.
- [ ] Có ít nhất 5 người đồng ý làm design partner và dành một buổi cài đặt.
- [ ] Ghi rõ họ đang thay thế/ghép với công cụ nào và vì sao họ sẽ thử TracePilot.
- [ ] Chọn đúng một lời hứa landing page và một primary flow cho beta.
- [ ] Đưa security analytics, metrics dashboard, Kubernetes, multi-project và AI ra
      khỏi beta trừ khi ít nhất 3 design partner cùng yêu cầu.

Exit gate: 5 design partner có lịch onboarding, không chỉ email “quan tâm”.

## Gate 2 — Cài đặt và activation

- [ ] Một lệnh khởi động local/dev từ clean machine; không yêu cầu sửa source.
- [ ] Một hướng dẫn production tối thiểu với image digest, PostgreSQL, TLS, secret,
      backup và upgrade path.
- [ ] Có preflight command kiểm tra port, DB, migration, key và readiness với lỗi có
      hành động sửa cụ thể.
- [ ] Không bắt người mới tự nghĩ API key; cung cấp lệnh sinh secret an toàn và giải
      thích bootstrap key chỉ hiển thị/lưu ở đâu.
- [ ] UI onboarding hiển thị tiến độ: connected → collector configured → first
      trace/log received → inspect trace.
- [ ] Có snippet copy/paste cho đúng 2 runtime design partner dùng nhiều nhất; chỉ
      thêm runtime thứ ba khi có yêu cầu thật.
- [ ] Có sample app hoặc `mockdata` tạo một trace lỗi và correlated log dễ nhận biết.
- [ ] Mọi empty state trong primary flow có một next action, không chỉ “No data”.
- [ ] Có troubleshooting theo symptom: 401, connection refused, no spans, clock
      skew, queue full và schema mismatch.
- [ ] Test 5 buổi cài đặt có screen recording/notes và đo time-to-first-trace.
- [ ] Median time-to-first-trace ≤ 10 phút; ít nhất 4/5 người hoàn thành mà
      maintainer không điều khiển máy.

Exit gate: setup success ≥ 80% và 4/5 design partner mở được trace/log của chính họ.

## Gate 3 — Giá trị của workflow

- [ ] Trace list giữ filter trong URL và hỗ trợ loading, empty, error, retry,
      pagination.
- [ ] Trace detail hiển thị cây/waterfall đúng cho nested, sibling và missing parent.
- [ ] Từ span lỗi tới correlated log không quá 3 thao tác.
- [ ] Người dùng phân biệt rõ dữ liệu đang live, bị trễ, bị drop hoặc hết retention.
- [ ] Có một workflow tạo/cập nhật incident tối thiểu nếu interview chứng minh cần;
      không buộc primary trace flow đi qua SIEM/security.
- [ ] Chạy 5 usability sessions bằng dữ liệu của user; ghi time-to-answer và điểm
      mắc kẹt, không hỏi chung “bạn có thích không?”.
- [ ] Ít nhất 3/5 user tìm được nguyên nhân của một case thật hoặc case tái hiện mà
      không cần đọc SQL/log của TracePilot.
- [ ] Mọi feature beta có owner, docs, error state và cách disable; capability không
      đạt điều này được gắn experimental hoặc bỏ khỏi thông điệp bán hàng.

Exit gate: ít nhất 3 user xác nhận TracePilot nhanh hơn hoặc đơn giản hơn workflow
hiện tại cho primary job.

## Gate 4 — Trust và vận hành beta

- [ ] Sửa mọi P0 trong `SOURCE_REVIEW_REFACTOR_PLAN.md`.
- [ ] Test cross-project/cross-environment cho toàn bộ endpoint thuộc primary flow.
- [ ] Test DB outage, queue saturation và shutdown không false ACK hoặc deadlock.
- [ ] Backup/restore drill có RPO/RTO đo được trên dataset gần beta.
- [ ] Upgrade N → N+1 và rollback application/migration được diễn tập.
- [ ] Retention mặc định, disk sizing và cảnh báo dung lượng được tài liệu hóa.
- [ ] Dashboard/alert tối thiểu cho readiness, ingest error, commit latency, queue,
      DB pool, disk và backup age.
- [ ] Security contact được theo dõi; có supported-version và vulnerability policy.
- [ ] Log audit xác nhận không lộ API key, database URL, webhook secret hoặc raw body
      nhạy cảm.
- [ ] Release có tag semver, changelog, checksum, SBOM, signature/provenance và image
      immutable digest.
- [ ] Docker/Compose production chỉ publish port cần thiết; container non-root,
      read-only và có CPU/RAM/log limits.
- [ ] Một design partner chạy liên tục 7 ngày mà không mất dữ liệu ngoài contract đã
      công bố.

Exit gate: 0 critical/high issue chưa có quyết định; restore và rollback có bằng
chứng; ít nhất một deployment ngoài đội chạy ổn 7 ngày.

## Gate 5 — Public beta và phân phối

- [ ] Repository/remote, package/image name và URL đều dùng nhất quán `tracepilot`.
- [ ] README trả lời trong màn hình đầu: dành cho ai, giải quyết gì, khác gì, chạy
      thử bằng lệnh nào và thấy kết quả gì.
- [ ] Có 3 ảnh thật: overview, trace waterfall + logs, incident hoặc security flow
      đang được hỗ trợ.
- [ ] Có video demo 2-3 phút từ clean install tới root cause; không dùng slide.
- [ ] Có trang so sánh trung thực: phù hợp khi nào, không phù hợp khi nào, giới hạn
      single replica/best-effort metrics.
- [ ] Có `CONTRIBUTING.md`, `SECURITY.md`, issue templates và discussion/support
      channel với SLA phản hồi beta.
- [ ] Public roadmap chỉ chứa outcome và giới hạn; backlog nội bộ không được trình
      bày như lời hứa release.
- [ ] Chuẩn bị một launch post tập trung vào problem, kiến trúc/durability trade-off
      và demo; không liệt kê feature dài.
- [ ] Chọn 2 kênh nơi ICP thực sự hiện diện và tham gia bằng case study; không spam
      cùng một bài lên mọi cộng đồng.
- [ ] Mỗi tuần mời trực tiếp 10 team đúng ICP dùng thử cho đến khi có 5 activated
      user; ghi lý do từ chối.
- [ ] Mọi issue beta được phản hồi trong 1 ngày làm việc và có owner/next step.

Exit gate: có 5 real users theo định nghĩa, không tính maintainer hoặc demo instance.

## Gate 6 — Retention trước khi mở rộng

- [ ] Liên hệ user ngày 1, 7 và 28; hỏi case đã dùng, outcome, blocker và công cụ họ
      vẫn phải quay lại.
- [ ] Ít nhất 3/5 activated user còn ingest và sử dụng ở tuần 4.
- [ ] Có ít nhất 2 case study/quote được user cho phép công bố.
- [ ] Top 3 blocker được xếp theo số user và mức chặn primary job, không theo độ thú
      vị kỹ thuật.
- [ ] Chỉ thêm feature khi gắn với một metric hoặc blocker cụ thể và có cách đo sau
      release.
- [ ] Xóa hoặc hạ experimental capability không có user sau hai vòng beta.
- [ ] Chốt support burden: số giờ/tuần, loại ticket và phần docs/product cần tự phục
      vụ tốt hơn.
- [ ] Quyết định vòng tiếp theo dựa trên dữ liệu: tăng activation, tăng retention,
      hay mở rộng ICP; chỉ chọn một mục tiêu chính.

Exit gate: week-4 retention ≥ 60% trong cohort 5 user đầu và có bằng chứng primary
job lặp lại. Nếu không đạt, tiếp tục sửa onboarding/workflow; chưa mở rộng tính năng.

## Bảng điều hành tuần

| Tuần | Leads đúng ICP | Onboarding đã đặt | Setup thành công | Activated 24h | Active tuần này | Blocker số 1 | Owner |
| --- | ---: | ---: | ---: | ---: | ---: | --- | --- |
| | | | | | | | |

## Definition of done: có người dùng thật

- [ ] 5 user ngoài đội đáp ứng định nghĩa real user.
- [ ] 4/5 tự cài và thấy trace đầu tiên trong 10 phút.
- [ ] 3/5 còn active ở tuần 4.
- [ ] 3 user hoàn thành primary job tốt hơn workflow trước đó.
- [ ] 2 user cho phép dùng quote/case study.
- [ ] Có release và runbook đủ để upgrade, restore và nhận hỗ trợ mà không phụ thuộc
      vào máy của maintainer.
