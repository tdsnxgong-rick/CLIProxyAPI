package trafficdump_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/logging"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/runtime/executor/helps"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/trafficdump"
)

func findSingleSessionDir(t *testing.T, tempDir string) string {
	t.Helper()
	dateEntries, err := os.ReadDir(tempDir)
	if err != nil || len(dateEntries) != 1 {
		t.Fatalf("expected 1 date directory, got %d (err: %v)", len(dateEntries), err)
	}

	dateDirPath := filepath.Join(tempDir, dateEntries[0].Name())
	sessionEntries, err := os.ReadDir(dateDirPath)
	if err != nil || len(sessionEntries) != 1 {
		t.Fatalf("expected 1 session directory in date folder, got %d (err: %v)", len(sessionEntries), err)
	}

	return filepath.Join(dateDirPath, sessionEntries[0].Name())
}

func TestTrafficDump_EndToEnd_NonStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tempDir := t.TempDir()

	cfg := &config.Config{
		SDKConfig: config.SDKConfig{
			DumpTraffic: config.DumpTrafficConfig{
				Enabled:  true,
				Dir:      tempDir,
				RawToken: true,
			},
		},
	}

	r := gin.New()
	r.Use(trafficdump.Middleware(cfg, ""))

	r.POST("/v1/chat/completions", func(c *gin.Context) {
		ctx := c.Request.Context()

		// Simulate upstream call
		upReqHeaders := http.Header{}
		upReqHeaders.Set("Authorization", "Bearer sk-upstream-secret")
		upReqHeaders.Set("Content-Type", "application/json")
		upBody := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`)

		helps.RecordAPIRequest(ctx, cfg, helps.UpstreamRequestLog{
			URL:     "https://api.openai.com/v1/chat/completions",
			Method:  http.MethodPost,
			Headers: upReqHeaders,
			Body:    upBody,
		})

		// Simulate upstream response
		upRespHeaders := http.Header{}
		upRespHeaders.Set("Content-Type", "application/json")
		upRespBody := []byte(`{"id":"chatcmpl-1","choices":[{"message":{"role":"assistant","content":"world"}}]}`)

		helps.RecordAPIResponseMetadata(ctx, cfg, http.StatusOK, upRespHeaders)
		helps.AppendAPIResponseChunk(ctx, cfg, upRespBody)

		// Return to client
		c.Header("Content-Type", "application/json")
		c.Data(http.StatusOK, "application/json", upRespBody)
	})

	clientReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`))
	clientReq.Header.Set("Content-Type", "application/json")
	clientReq.Header.Set("X-Request-ID", "e2e-nonstream-req")
	clientReq.Header.Set("Authorization", "Bearer sk-client-token")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, clientReq)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code: %d", rec.Code)
	}

	sessionDirPath := findSingleSessionDir(t, tempDir)

	// 1. client-request.txt
	cReq, err := os.ReadFile(filepath.Join(sessionDirPath, "client-request.txt"))
	if err != nil {
		t.Fatalf("failed to read client-request.txt: %v", err)
	}
	if !strings.Contains(string(cReq), "POST /v1/chat/completions HTTP/1.1") || !strings.Contains(string(cReq), "sk-client-token") {
		t.Errorf("invalid client-request.txt content: %s", string(cReq))
	}

	// 2. client-response.txt
	cResp, err := os.ReadFile(filepath.Join(sessionDirPath, "client-response.txt"))
	if err != nil {
		t.Fatalf("failed to read client-response.txt: %v", err)
	}
	if !strings.Contains(string(cResp), "HTTP/1.1 200 OK") || !strings.Contains(string(cResp), "chatcmpl-1") {
		t.Errorf("invalid client-response.txt content: %s", string(cResp))
	}

	// 3. upstream-request.txt
	uReq, err := os.ReadFile(filepath.Join(sessionDirPath, "upstream-request.txt"))
	if err != nil {
		t.Fatalf("failed to read upstream-request.txt: %v", err)
	}
	if !strings.Contains(string(uReq), "POST https://api.openai.com/v1/chat/completions HTTP/1.1") || !strings.Contains(string(uReq), "sk-upstream-secret") {
		t.Errorf("invalid upstream-request.txt content: %s", string(uReq))
	}

	// 4. upstream-response.txt
	uResp, err := os.ReadFile(filepath.Join(sessionDirPath, "upstream-response.txt"))
	if err != nil {
		t.Fatalf("failed to read upstream-response.txt: %v", err)
	}
	if !strings.Contains(string(uResp), "HTTP/1.1 200 OK") || !strings.Contains(string(uResp), "chatcmpl-1") {
		t.Errorf("invalid upstream-response.txt content: %s", string(uResp))
	}
}

func TestTrafficDump_EndToEnd_StreamingAndRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tempDir := t.TempDir()

	cfg := &config.Config{
		SDKConfig: config.SDKConfig{
			DumpTraffic: config.DumpTrafficConfig{
				Enabled:  true,
				Dir:      tempDir,
				RawToken: false, // masked
			},
		},
	}

	r := gin.New()
	r.Use(trafficdump.Middleware(cfg, ""))

	r.POST("/v1/chat/completions", func(c *gin.Context) {
		ctx := c.Request.Context()

		// Attempt 1: fails
		upReqHeaders := http.Header{}
		upReqHeaders.Set("Authorization", "Bearer sk-upstream-attempt-1")
		helps.RecordAPIRequest(ctx, cfg, helps.UpstreamRequestLog{
			URL:     "https://api.openai.com/v1/chat/completions",
			Method:  http.MethodPost,
			Headers: upReqHeaders,
			Body:    []byte(`{"stream":true}`),
		})
		helps.RecordAPIResponseError(ctx, cfg, fmt.Errorf("connection reset by peer"))

		// Attempt 2: succeeds with streaming
		upReqHeaders2 := http.Header{}
		upReqHeaders2.Set("Authorization", "Bearer sk-upstream-attempt-2")
		helps.RecordAPIRequest(ctx, cfg, helps.UpstreamRequestLog{
			URL:     "https://api.openai.com/v1/chat/completions",
			Method:  http.MethodPost,
			Headers: upReqHeaders2,
			Body:    []byte(`{"stream":true}`),
		})

		respHeaders := http.Header{}
		respHeaders.Set("Content-Type", "text/event-stream")
		helps.RecordAPIResponseMetadata(ctx, cfg, http.StatusOK, respHeaders)

		c.Header("Content-Type", "text/event-stream")
		c.Status(http.StatusOK)

		chunk1 := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		chunk2 := []byte("data: [DONE]\n\n")

		helps.AppendAPIResponseChunk(ctx, cfg, chunk1)
		_, _ = c.Writer.Write(chunk1)
		c.Writer.Flush()

		helps.AppendAPIResponseChunk(ctx, cfg, chunk2)
		_, _ = c.Writer.Write(chunk2)
		c.Writer.Flush()
	})

	clientReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"stream":true}`))
	clientReq.Header.Set("Authorization", "Bearer sk-client-raw-secret")
	clientReq.Header.Set("X-Request-ID", "streaming-retry-req")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, clientReq)

	sessionDirPath := findSingleSessionDir(t, tempDir)

	// Verify attempt 1 files (named upstream-request.txt and upstream-response.txt)
	uReq1, err := os.ReadFile(filepath.Join(sessionDirPath, "upstream-request.txt"))
	if err != nil {
		t.Fatalf("failed to read upstream-request.txt: %v", err)
	}
	if strings.Contains(string(uReq1), "sk-upstream-attempt-1") {
		t.Errorf("token should be masked when raw-token is false")
	}

	uResp1, err := os.ReadFile(filepath.Join(sessionDirPath, "upstream-response.txt"))
	if err != nil {
		t.Fatalf("failed to read upstream-response.txt: %v", err)
	}
	if !strings.Contains(string(uResp1), "connection reset by peer") {
		t.Errorf("expected error in upstream-response.txt: %s", string(uResp1))
	}

	// Verify attempt 2 files (named upstream-request-2.txt and upstream-response-2.txt)
	uReq2, err := os.ReadFile(filepath.Join(sessionDirPath, "upstream-request-2.txt"))
	if err != nil {
		t.Fatalf("failed to read upstream-request-2.txt: %v", err)
	}
	if len(uReq2) == 0 {
		t.Errorf("upstream-request-2.txt is empty")
	}

	uResp2, err := os.ReadFile(filepath.Join(sessionDirPath, "upstream-response-2.txt"))
	if err != nil {
		t.Fatalf("read upstream-response-2.txt: %v", err)
	}
	if !strings.Contains(string(uResp2), "Hello") || !strings.Contains(string(uResp2), "[DONE]") {
		t.Errorf("upstream-response-2.txt missing streaming chunks: %s", string(uResp2))
	}

	// Verify client response has all streaming chunks
	cResp, err := os.ReadFile(filepath.Join(sessionDirPath, "client-response.txt"))
	if err != nil {
		t.Fatalf("failed to read client-response.txt: %v", err)
	}
	if !strings.Contains(string(cResp), "Hello") || !strings.Contains(string(cResp), "[DONE]") {
		t.Errorf("client-response.txt missing streaming chunks: %s", string(cResp))
	}
}

func TestTrafficDump_Concurrency(t *testing.T) {
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
	r.Use(trafficdump.Middleware(cfg, ""))

	r.POST("/v1/chat/completions", func(c *gin.Context) {
		reqID := logging.GetGinRequestID(c)
		ctx := c.Request.Context()

		helps.RecordAPIRequest(ctx, cfg, helps.UpstreamRequestLog{
			URL:     "https://api.openai.com/v1/chat/completions",
			Method:  http.MethodPost,
			Headers: http.Header{"X-Req": []string{reqID}},
			Body:    []byte(fmt.Sprintf(`{"req":"%s"}`, reqID)),
		})

		helps.RecordAPIResponseMetadata(ctx, cfg, http.StatusOK, http.Header{"X-Resp": []string{reqID}})
		helps.AppendAPIResponseChunk(ctx, cfg, []byte(fmt.Sprintf(`{"resp":"%s"}`, reqID)))

		c.String(http.StatusOK, "ok-"+reqID)
	})

	const concurrentCount = 20
	var wg sync.WaitGroup
	wg.Add(concurrentCount)

	for i := 0; i < concurrentCount; i++ {
		reqIndex := i
		go func() {
			defer wg.Done()
			reqID := fmt.Sprintf("concurrent-req-%d", reqIndex)
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(fmt.Sprintf("body-%d", reqIndex)))
			req.Header.Set("X-Request-ID", reqID)

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("request %d failed with status %d", reqIndex, w.Code)
			}
		}()
	}

	wg.Wait()

	dateEntries, err := os.ReadDir(tempDir)
	if err != nil || len(dateEntries) == 0 {
		t.Fatalf("failed to read tempDir date entries: %v", err)
	}

	var totalSessionDirs int
	for _, dEntry := range dateEntries {
		if !dEntry.IsDir() {
			continue
		}
		sEntries, err := os.ReadDir(filepath.Join(tempDir, dEntry.Name()))
		if err != nil {
			continue
		}
		totalSessionDirs += len(sEntries)
		for _, sEntry := range sEntries {
			sDirPath := filepath.Join(tempDir, dEntry.Name(), sEntry.Name())
			files, err := os.ReadDir(sDirPath)
			if err != nil {
				t.Fatalf("failed to read dir %s: %v", sDirPath, err)
			}
			if len(files) < 4 {
				t.Errorf("session dir %s has %d files, expected at least 4", sEntry.Name(), len(files))
			}
		}
	}

	if totalSessionDirs != concurrentCount {
		t.Fatalf("expected %d session directories in total, got %d", concurrentCount, totalSessionDirs)
	}
}
