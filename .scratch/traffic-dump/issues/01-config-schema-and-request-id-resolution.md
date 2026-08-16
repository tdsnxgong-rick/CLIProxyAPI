# 01 — Config Schema & Request ID Header Resolution

**What to build:**
Support the `dump-traffic` configuration block in `config.yaml` and resolve client-provided `X-Request-ID` headers to drive end-to-end request tracing. When clients include an `X-Request-ID` header, the proxy adopts it as the canonical session Request ID instead of generating an anonymous random ID.

**Blocked by:** None — can start immediately

**Status:** closed

- [x] `dump-traffic` section with `enabled`, `dir` (defaults to `logs/traffic`), and `raw-token` (boolean) is parsed and validated from `config.yaml`
- [x] Gin request logger and context extraction prioritize incoming `X-Request-ID` headers over newly generated 8-character hex IDs
- [x] Unit tests verify config parsing, default values, and Request ID extraction from client headers
