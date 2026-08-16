# 04 — End-to-End Integration Verification & Concurrency Tests

**What to build:**
End-to-end integration and concurrency test suite testing the full proxy server lifecycle with traffic dumping enabled against mock upstream providers. Verifies that all 4 files (`client-request.txt`, `client-response.txt`, `upstream-request.txt`, `upstream-response.txt`) are reliably generated, structurally complete, concurrency-safe, and free of file descriptor or memory leaks.

**Blocked by:** 03 — Upstream Traffic Capture (upstream-request.txt & upstream-response.txt)

**Status:** closed

- [x] End-to-end test verifying a standard non-streaming AI completion request produces all 4 expected traffic dump files
- [x] End-to-end test verifying an SSE streaming completion request writes real-time chunks to both response files
- [x] End-to-end test verifying upstream failure and retry creates indexed upstream attempt files alongside client files
- [x] Concurrency test verifying parallel requests maintain clean directory isolation without race conditions or file handle leaks
- [x] Compilation verification via `go build ./cmd/server` passes cleanly
