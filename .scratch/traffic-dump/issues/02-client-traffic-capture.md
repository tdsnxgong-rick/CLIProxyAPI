# 02 — Client Traffic Capture (client-request.txt & client-response.txt)

**What to build:**
When `dump-traffic.enabled` is true, automatically create an isolated session directory (`<dir>/<timestamp>-<request-id>/`) upon receiving a client request. Immediately write `client-request.txt` containing the full raw HTTP request line, headers, and body. As the proxy produces or streams the client response, record `client-response.txt` with the HTTP status line, headers, and full payload (including real-time streaming SSE chunks), guaranteeing reliable flush upon completion or client disconnect.

**Blocked by:** 01 — Config Schema & Request ID Header Resolution

**Status:** closed

- [x] Directory `<dump-dir>/<timestamp>-<request-id>/` is automatically initialized on client request entry
- [x] `client-request.txt` is written in valid HTTP wire format (request line + headers + blank line + raw body)
- [x] `client-response.txt` is written with HTTP status line, headers, and complete response content
- [x] Streaming SSE responses are written chunk-by-chunk in real time to capture partial output upon client cancellation
- [x] Unit/middleware tests verify file creation, wire format accuracy, and graceful cleanup of resources
