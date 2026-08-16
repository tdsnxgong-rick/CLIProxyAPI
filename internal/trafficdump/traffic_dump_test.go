package trafficdump

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

func TestTrafficDumpSession_ClientAndUpstreamFiles(t *testing.T) {
	tempDir := t.TempDir()
	now := time.Date(2026, 8, 16, 15, 30, 0, 123000000, time.UTC)
	requestID := "testreq1"

	session, err := NewSession(tempDir, requestID, now, false)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Verify path structure: tempDir/2026-08-16/15-30-00-123-testreq1
	expectedDateFolder := "2026-08-16"
	expectedTimeFolder := "15-30-00-123-testreq1"
	expectedSessionDir := filepath.Join(tempDir, expectedDateFolder, expectedTimeFolder)

	if session.GetSessionDir() != expectedSessionDir {
		t.Errorf("expected session dir %q, got %q", expectedSessionDir, session.GetSessionDir())
	}

	// 1. Write client request
	clientHeaders := http.Header{}
	clientHeaders.Set("Content-Type", "application/json")
	clientHeaders.Set("Authorization", "Bearer sk-secret123456789")
	clientBody := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`)

	if err := session.WriteClientRequest(http.MethodPost, "/v1/chat/completions", "HTTP/1.1", clientHeaders, clientBody); err != nil {
		t.Fatalf("WriteClientRequest failed: %v", err)
	}

	// 2. Write client response
	respHeaders := http.Header{}
	respHeaders.Set("Content-Type", "text/event-stream")
	if err := session.WriteClientResponseHeaders(http.StatusOK, "HTTP/1.1", respHeaders); err != nil {
		t.Fatalf("WriteClientResponseHeaders failed: %v", err)
	}
	if err := session.WriteClientResponseBodyChunk([]byte("data: {\"choices\":[]}\n\n")); err != nil {
		t.Fatalf("WriteClientResponseBodyChunk failed: %v", err)
	}

	// 3. Write upstream request attempt 1
	upHeaders := http.Header{}
	upHeaders.Set("Authorization", "Bearer sk-upstream-key-999")
	if err := session.WriteUpstreamRequest(1, http.MethodPost, "https://api.openai.com/v1/chat/completions", "HTTP/1.1", upHeaders, clientBody); err != nil {
		t.Fatalf("WriteUpstreamRequest 1 failed: %v", err)
	}

	// 4. Write upstream response attempt 1
	if err := session.WriteUpstreamResponseHeaders(1, http.StatusOK, "HTTP/1.1", respHeaders); err != nil {
		t.Fatalf("WriteUpstreamResponseHeaders 1 failed: %v", err)
	}
	if err := session.WriteUpstreamResponseBodyChunk(1, []byte("data: {\"choices\":[]}\n\n")); err != nil {
		t.Fatalf("WriteUpstreamResponseBodyChunk 1 failed: %v", err)
	}

	// 5. Write upstream retry attempt 2
	if err := session.WriteUpstreamRequest(2, http.MethodPost, "https://api.openai.com/v1/chat/completions", "HTTP/1.1", upHeaders, clientBody); err != nil {
		t.Fatalf("WriteUpstreamRequest 2 failed: %v", err)
	}
	if err := session.WriteUpstreamError(2, os.ErrDeadlineExceeded); err != nil {
		t.Fatalf("WriteUpstreamError 2 failed: %v", err)
	}

	if err := session.Close(); err != nil {
		t.Fatalf("session.Close failed: %v", err)
	}

	sessionDir := session.GetSessionDir()

	// Verify client-request.txt
	clientReqBytes, err := os.ReadFile(filepath.Join(sessionDir, "client-request.txt"))
	if err != nil {
		t.Fatalf("read client-request.txt: %v", err)
	}
	clientReqStr := string(clientReqBytes)
	if !strings.HasPrefix(clientReqStr, "POST /v1/chat/completions HTTP/1.1\n") {
		t.Errorf("unexpected client-request header: %s", clientReqStr)
	}
	// Verify token is masked by default
	if strings.Contains(clientReqStr, "sk-secret123456789") {
		t.Errorf("expected token to be masked in client-request.txt")
	}
	if !strings.Contains(clientReqStr, `"model":"gpt-4o"`) {
		t.Errorf("expected body in client-request.txt")
	}

	// Verify client-response.txt
	clientRespBytes, err := os.ReadFile(filepath.Join(sessionDir, "client-response.txt"))
	if err != nil {
		t.Fatalf("read client-response.txt: %v", err)
	}
	if !strings.Contains(string(clientRespBytes), "HTTP/1.1 200 OK") || !strings.Contains(string(clientRespBytes), "data: {\"choices\":[]}\n\n") {
		t.Errorf("unexpected client-response.txt content: %s", string(clientRespBytes))
	}

	// Verify upstream-request.txt
	upReqBytes, err := os.ReadFile(filepath.Join(sessionDir, "upstream-request.txt"))
	if err != nil {
		t.Fatalf("read upstream-request.txt: %v", err)
	}
	if !strings.Contains(string(upReqBytes), "POST https://api.openai.com/v1/chat/completions HTTP/1.1") {
		t.Errorf("unexpected upstream-request.txt content: %s", string(upReqBytes))
	}

	// Verify upstream-response.txt
	upRespBytes, err := os.ReadFile(filepath.Join(sessionDir, "upstream-response.txt"))
	if err != nil {
		t.Fatalf("read upstream-response.txt: %v", err)
	}
	if !strings.Contains(string(upRespBytes), "HTTP/1.1 200 OK") {
		t.Errorf("unexpected upstream-response.txt content: %s", string(upRespBytes))
	}

	// Verify retry attempt 2 files
	upReq2Bytes, err := os.ReadFile(filepath.Join(sessionDir, "upstream-request-2.txt"))
	if err != nil {
		t.Fatalf("read upstream-request-2.txt: %v", err)
	}
	if len(upReq2Bytes) == 0 {
		t.Errorf("upstream-request-2.txt is empty")
	}

	upResp2Bytes, err := os.ReadFile(filepath.Join(sessionDir, "upstream-response-2.txt"))
	if err != nil {
		t.Fatalf("read upstream-response-2.txt: %v", err)
	}
	if !strings.Contains(string(upResp2Bytes), "[UPSTREAM ERROR:") {
		t.Errorf("expected error in upstream-response-2.txt: %s", string(upResp2Bytes))
	}
}

func TestTrafficDumpSession_RawToken(t *testing.T) {
	tempDir := t.TempDir()
	now := time.Now()
	requestID := "rawreq1"

	session, err := NewSession(tempDir, requestID, now, true) // rawToken = true
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	clientHeaders := http.Header{}
	clientHeaders.Set("Authorization", "Bearer sk-plain-token-123")
	if err := session.WriteClientRequest(http.MethodGet, "/test", "HTTP/1.1", clientHeaders, nil); err != nil {
		t.Fatalf("WriteClientRequest failed: %v", err)
	}
	_ = session.Close()

	content, err := os.ReadFile(filepath.Join(session.GetSessionDir(), "client-request.txt"))
	if err != nil {
		t.Fatalf("read client-request.txt: %v", err)
	}
	if !strings.Contains(string(content), "sk-plain-token-123") {
		t.Errorf("expected raw token preserved, got: %s", string(content))
	}
}

func TestTrafficDumpMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tempDir := t.TempDir()

	cfg := &config.Config{
		SDKConfig: config.SDKConfig{
			DumpTraffic: config.DumpTrafficConfig{
				Enabled: true,
				Dir:     tempDir,
			},
		},
	}

	r := gin.New()
	r.Use(Middleware(cfg, ""))
	r.POST("/v1/test", func(c *gin.Context) {
		body, _ := c.GetRawData()
		c.Header("X-Custom-Resp", "test-val")
		c.String(http.StatusOK, "echo: "+string(body))
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/test", bytes.NewBufferString("payload-123"))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("X-Request-ID", "custom-mw-req-id")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("handler returned status %d", w.Code)
	}
	if w.Body.String() != "echo: payload-123" {
		t.Fatalf("unexpected handler body: %s", w.Body.String())
	}

	dateEntries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("read tempDir: %v", err)
	}
	if len(dateEntries) != 1 {
		t.Fatalf("expected 1 date dir, got %d", len(dateEntries))
	}

	dateDirPath := filepath.Join(tempDir, dateEntries[0].Name())
	sessionEntries, err := os.ReadDir(dateDirPath)
	if err != nil || len(sessionEntries) != 1 {
		t.Fatalf("expected 1 session dir in date folder, got %d", len(sessionEntries))
	}

	sessionDirName := sessionEntries[0].Name()
	if !strings.Contains(sessionDirName, "custom-mw-req-id") {
		t.Errorf("expected session dir name to contain request ID, got %s", sessionDirName)
	}

	sessionDirPath := filepath.Join(dateDirPath, sessionDirName)
	clientReqBytes, err := os.ReadFile(filepath.Join(sessionDirPath, "client-request.txt"))
	if err != nil {
		t.Fatalf("read client-request.txt: %v", err)
	}
	if !strings.Contains(string(clientReqBytes), "payload-123") {
		t.Errorf("client-request.txt missing body: %s", string(clientReqBytes))
	}

	clientRespBytes, err := os.ReadFile(filepath.Join(sessionDirPath, "client-response.txt"))
	if err != nil {
		t.Fatalf("read client-response.txt: %v", err)
	}
	if !strings.Contains(string(clientRespBytes), "echo: payload-123") {
		t.Errorf("client-response.txt missing body: %s", string(clientRespBytes))
	}
}

func TestTrafficDumpCleaner_RetentionDaysAndSizeLimit(t *testing.T) {
	tempDir := t.TempDir()

	// Create 3 date folders
	// Folder 1: 40 days old (should be purged by 30-day retention)
	oldDate := time.Now().AddDate(0, 0, -40).Format("2006-01-02")
	oldSessionDir := filepath.Join(tempDir, oldDate, "12-00-00-000-oldreq")
	if err := os.MkdirAll(oldSessionDir, 0755); err != nil {
		t.Fatalf("mkdir old session: %v", err)
	}
	_ = os.WriteFile(filepath.Join(oldSessionDir, "client-request.txt"), bytes.Repeat([]byte("A"), 100), 0644)
	oldTime := time.Now().AddDate(0, 0, -40)
	_ = os.Chtimes(oldSessionDir, oldTime, oldTime)
	_ = os.Chtimes(filepath.Join(oldSessionDir, "client-request.txt"), oldTime, oldTime)

	// Folder 2: 1 day old, 500 KB
	recentDate := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	recentSession1 := filepath.Join(tempDir, recentDate, "10-00-00-000-req1")
	if err := os.MkdirAll(recentSession1, 0755); err != nil {
		t.Fatalf("mkdir recent session 1: %v", err)
	}
	_ = os.WriteFile(filepath.Join(recentSession1, "client-request.txt"), bytes.Repeat([]byte("B"), 500*1024), 0644)
	t1 := time.Now().AddDate(0, 0, -1)
	_ = os.Chtimes(recentSession1, t1, t1)
	_ = os.Chtimes(filepath.Join(recentSession1, "client-request.txt"), t1, t1)

	// Folder 3: today, 700 KB
	todayDate := time.Now().Format("2006-01-02")
	recentSession2 := filepath.Join(tempDir, todayDate, "11-00-00-000-req2")
	if err := os.MkdirAll(recentSession2, 0755); err != nil {
		t.Fatalf("mkdir recent session 2: %v", err)
	}
	_ = os.WriteFile(filepath.Join(recentSession2, "client-request.txt"), bytes.Repeat([]byte("C"), 700*1024), 0644)

	// Enforce 30-day retention and 1MB limit (1MB = 1048576 bytes)
	// Total size of recentSession1 (500KB) + recentSession2 (700KB) = 1.2MB > 1MB
	// Expected: oldSession purged by retention, recentSession1 purged by size limit, recentSession2 kept!
	deleted, err := EnforceTrafficLimits(tempDir, 30, 1) // 1MB limit
	if err != nil {
		t.Fatalf("EnforceTrafficLimits failed: %v", err)
	}
	if deleted < 2 {
		t.Errorf("expected at least 2 sessions deleted, got %d", deleted)
	}

	// Verify old session and empty date folder were removed
	if _, err := os.Stat(oldSessionDir); !os.IsNotExist(err) {
		t.Errorf("expected old session folder to be removed")
	}
	if _, err := os.Stat(filepath.Join(tempDir, oldDate)); !os.IsNotExist(err) {
		t.Errorf("expected empty date folder to be removed")
	}

	// Verify newest session is retained
	if _, err := os.Stat(recentSession2); os.IsNotExist(err) {
		t.Errorf("expected newest session folder to be retained")
	}
}
