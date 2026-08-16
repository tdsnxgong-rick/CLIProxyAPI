# 03 — Upstream Traffic Capture (upstream-request.txt & upstream-response.txt)

**What to build:**
Capture the exact outbound HTTP request dispatched to the upstream AI provider in `upstream-request.txt` and the upstream response in `upstream-response.txt` within the active session folder. When multi-attempt retries or model failovers occur, record subsequent attempts with sequence indices (e.g. `upstream-request-2.txt`, `upstream-response-2.txt`). Faithfully record all upstream failures, timeouts, and non-200 HTTP statuses. Honor `raw-token` setting to emit unmasked Authorization headers when explicitly requested.

**Blocked by:** 02 — Client Traffic Capture (client-request.txt & client-response.txt)

**Status:** closed

- [x] Outbound upstream HTTP request line, headers, and translated body are captured in `upstream-request.txt`
- [x] Upstream response status line, headers, and body/SSE chunks are captured in `upstream-response.txt`
- [x] Multiple upstream attempts (retries / key rotation) generate sequentially indexed files (`-1.txt`, `-2.txt`)
- [x] Upstream network errors, timeouts, and 4xx/5xx responses are captured in `upstream-response.txt` without omission
- [x] Sensitive headers (e.g. `Authorization`, `api-key`) remain unmasked when `raw-token: true` and masked when `false`
- [x] Tests verify multi-attempt capture, error recording, and unmasked token handling
