# Replay

Replay is a later vertical slice and remains disabled after Phase 2. It cannot be enabled before safe bounded capture, redaction, artifact authorization, and adversarial SSRF tests pass.

The lifecycle is incident -> redacted request artifact -> target policy -> bounded execution -> bounded result artifact -> semantic comparison -> audit record.

Default policy strips `Authorization`, `Proxy-Authorization`, `Cookie`, `Set-Cookie`, `X-API-Key`, and configured sensitive headers. It accepts only explicitly allowed HTTP/HTTPS targets, rejects URL userinfo, validates all DNS answers and connection IPs, blocks link-local/cloud metadata destinations, disables implicit redirects, validates each redirect, and applies request/response/total limits.

Deployment egress policy must reinforce application checks. Production credentials are never replayed automatically and production target ranges remain denied by default. See [security.md](security.md).
