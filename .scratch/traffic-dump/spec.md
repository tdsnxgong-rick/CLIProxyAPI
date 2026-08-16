# Specification: Traffic Dump

## Problem Statement

When debugging upstream AI provider integration issues, protocol mismatches, latency anomalies, or unexpected errors, developers and operators cannot inspect the raw, unadulterated wire-level HTTP traffic exchanged during a proxy session.

The existing request logger records summarized or single-file diagnostic logs, but lacks a dedicated traffic capture mechanism that reliably splits and dumps the exact wire HTTP payloads into separate, human-readable files (`client-request.txt`, `client-response.txt`, `upstream-request.txt`, and `upstream-response.txt`), regardless of whether the upstream connection succeeds, fails, or streams chunks over SSE.

## Solution

Introduce a configurable **Traffic Dump** capability. When enabled in the configuration file:
- Every client interaction creates an isolated directory partitioned by date and millisecond timestamp with **Request ID** (`logs/traffic/<YYYY-MM-DD>/<HH-mm-ss-SSS>-<request-id>/`).
- Four distinct raw HTTP wire-format files are produced per request cycle: `client-request.txt`, `client-response.txt`, `upstream-request.txt`, and `upstream-response.txt`.
- Streaming responses (SSE) are streamed to disk in real-time to guarantee that aborted connections and early errors are faithfully captured.
- Upstream retries or multi-attempt failovers are recorded with sequence indicators (e.g. `upstream-request-2.txt`).
- The **Request ID** is extracted from the client's `X-Request-ID` header if provided, or generated as an 8-character hex string.
- An optional raw token mode allows capturing genuine authorization headers without masking for local diagnostics.
- An integrated background retention cleaner automatically enforces age and disk size limits to prevent log directory growth.

## User Stories

1. As an API developer debugging prompt engineering issues, I want `client-request.txt` to contain the exact HTTP request line, headers, and body received by the proxy, so that I can verify the client payload without distortion.
2. As a backend engineer diagnosing model generation errors, I want `client-response.txt` to faithfully record every byte returned to the client, so that I can see the exact output delivered to the end-user.
3. As a developer troubleshooting provider translation discrepancies, I want `upstream-request.txt` to capture the exact translated HTTP request sent to the AI provider, so that I can inspect the final payload and provider-specific parameters.
4. As an engineer investigating provider outages or 5xx errors, I want `upstream-response.txt` to record the provider's exact HTTP status code, response headers, and error payload even when the provider fails, so that I can analyze the root cause.
5. As a developer testing streaming completions, I want streaming SSE response chunks written to disk in real time, so that partial output is preserved even if the client disconnects prematurely.
6. As an engineer evaluating failover logic, I want multiple upstream attempts (e.g. key rotation or model fallback) saved as distinct sequential files (e.g. `upstream-request-1.txt`, `upstream-request-2.txt`), so that I can inspect every failed and successful attempt.
7. As an operator tracking requests across microservices, I want the system to honor the incoming `X-Request-ID` header as the directory identifier, so that I can correlate proxy dumps with client-side traces.
8. As an operator running ad-hoc local debugging sessions, I want an optional configuration flag to disable sensitive token masking, so that I can verify raw authorization headers and signature formats.
9. As a system administrator managing disk usage, I want an explicit configuration toggle and directory path setting for traffic dumps, so that capture is only activated on demand and directed to the intended volume.
10. As an API consumer encountering network timeouts, I want upstream connection failure details captured in the upstream response log, so that I know whether the timeout occurred during connection handshake, header transfer, or body read.

## Implementation Decisions

1. **Configuration Schema**:
   - Add a dedicated `dump-traffic` configuration block containing:
     - `enabled` (boolean): Global toggle for traffic dumping.
     - `dir` (string): Target root directory for dumped traffic sessions (defaults to `logs/traffic`).
     - `raw-token` (boolean): When true, logs credentials verbatim without masking.
     - `max-retention-days` (int): Number of days to retain traffic logs before automatic removal (0 to disable).
     - `max-total-size-mb` (int): Maximum total storage space in MB allocated for all traffic dumps (0 to disable).
     - `clean-interval-hours` (int): Interval in hours between background retention sweeps (defaults to 1).

2. **Request ID Sourcing**:
   - Check incoming client headers for `X-Request-ID`. If non-empty, sanitize and use it as the session Request ID.
   - If `X-Request-ID` is missing, generate an 8-character hexadecimal identifier.

3. **Storage & Directory Structure**:
   - For every client request handled when the feature is enabled, create an isolated directory: `<dump-dir>/<YYYY-MM-DD>/<HH-mm-ss-SSS>-<request-id>/`.
   - File outputs within this directory:
     - `client-request.txt`: Captured immediately upon receiving the client request.
     - `client-response.txt`: Appended and finalized as client response headers and chunks are transmitted.
     - `upstream-request.txt` (and `-N.txt` on retry): Written immediately before dispatching each upstream request.
     - `upstream-response.txt` (and `-N.txt` on retry): Written as upstream response headers and stream chunks are received, or populated with error details upon network/HTTP failure.

4. **Wire Format Protocol**:
   - Each file adheres strictly to HTTP wire format:
     - First line: HTTP Method/URL or HTTP Status line (e.g. `POST /v1/chat/completions HTTP/1.1` or `HTTP/1.1 200 OK`).
     - Headers: Formatted as `Key: Value\r\n`.
     - Separator: Blank line (`\r\n` or `\n`).
     - Body / Stream Chunks: Exact raw bytes (including SSE `data: ...\n\n` event blocks).

5. **Streaming & Asynchronous Flushing**:
   - Writers for streaming responses flush data synchronously or via buffered non-blocking background workers ensuring file handles close reliably upon request completion or cancellation.

6. **Error Resiliency**:
   - Upstream transport errors, context cancellations, and non-200 status codes bypass no logging stages; failure details are rendered directly into the respective `upstream-response.txt`.

## Testing Decisions

- **Testing Seam**:
  - Test at the highest possible seam: the HTTP Proxy Server integration test seam. An automated test spins up a test proxy instance with `dump-traffic` enabled and points it to a mock upstream HTTP server.
- **Verification Criteria**:
  - Verify directory creation and naming against timestamp and Request ID.
  - Verify `client-request.txt` matching sent request headers and body.
  - Verify `client-response.txt` matching expected proxy response.
  - Verify `upstream-request.txt` matching translated payload delivered to mock server.
  - Verify `upstream-response.txt` matching mock server response.
  - Verify multi-attempt retry file creation (`-1.txt`, `-2.txt`) when mock upstream returns 500 followed by 200.
  - Verify SSE streaming chunks written properly in real-time.
  - Verify `raw-token: true` vs `raw-token: false` header masking behavior.
- **Prior Art**:
  - Existing proxy test suites under `internal/api/` and `test/` verifying Gin middleware, response writer wrappers, and mock server executions.

## Out of Scope

- Real-time Web UI / Dashboard for viewing traffic dumps.
- Automatic compression (gzip/zstd) of individual dumped traffic files.
- Binary payload packet capture (PCAP) decoding.

## Further Notes

- The traffic dump mechanism is completely decoupled from the existing single-file `request-log` feature and can be operated independently or concurrently.
