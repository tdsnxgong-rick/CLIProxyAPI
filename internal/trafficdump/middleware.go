package trafficdump

import (
	"bytes"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/logging"
	log "github.com/sirupsen/logrus"
)

// Middleware returns a Gin middleware that dumps client request and response wire traffic to disk.
func Middleware(cfg *config.Config, configDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if cfg == nil || !cfg.DumpTraffic.Enabled || c.Request == nil {
			c.Next()
			return
		}

		path := c.Request.URL.Path
		// Skip management endpoints to avoid leaking management console traffic
		if strings.HasPrefix(path, "/v0/management") || strings.HasPrefix(path, "/management") {
			c.Next()
			return
		}

		requestID := logging.GetGinRequestID(c)
		if requestID == "" {
			requestID = logging.ExtractOrGenerateRequestID(c.Request)
			logging.SetGinRequestID(c, requestID)
		}

		baseDir := cfg.DumpTraffic.Dir
		if baseDir == "" {
			baseDir = "logs/traffic"
		}
		if !filepath.IsAbs(baseDir) && configDir != "" {
			baseDir = filepath.Join(configDir, baseDir)
		}

		session, err := NewSession(baseDir, requestID, time.Now(), cfg.DumpTraffic.RawToken)
		if err != nil {
			log.WithError(err).Warn("failed to initialize traffic dump session")
			c.Next()
			return
		}
		defer func() {
			if errClose := session.Close(); errClose != nil {
				log.WithError(errClose).Warn("failed to close traffic dump session")
			}
		}()

		// Capture client request body
		var bodyBytes []byte
		if c.Request.Body != nil && c.Request.Body != http.NoBody {
			var errRead error
			bodyBytes, errRead = io.ReadAll(c.Request.Body)
			if errRead == nil {
				c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			} else {
				log.WithError(errRead).Warn("failed to read request body for traffic dump")
			}
		}

		uri := c.Request.URL.RequestURI()
		if uri == "" {
			uri = c.Request.URL.Path
		}
		if errReq := session.WriteClientRequest(c.Request.Method, uri, c.Request.Proto, c.Request.Header, bodyBytes); errReq != nil {
			log.WithError(errReq).Warn("failed to write client-request.txt")
		}

		// Inject session into context
		SetGinSession(c, session)
		ctx := WithContext(c.Request.Context(), session)
		c.Request = c.Request.WithContext(ctx)

		// Wrap response writer to capture client response status, headers, and body chunks in real-time
		writerWrapper := &clientResponseWriterWrapper{
			ResponseWriter: c.Writer,
			session:        session,
			proto:          c.Request.Proto,
		}
		c.Writer = writerWrapper

		c.Next()

		// Ensure headers are written even if body was never written
		writerWrapper.ensureHeadersWritten()
	}
}

type clientResponseWriterWrapper struct {
	gin.ResponseWriter
	session        *Session
	proto          string
	headersWritten bool
}

func (w *clientResponseWriterWrapper) WriteHeader(statusCode int) {
	w.ensureHeadersWrittenWithStatus(statusCode)
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *clientResponseWriterWrapper) Write(data []byte) (int, error) {
	w.ensureHeadersWritten()
	if w.session != nil && len(data) > 0 {
		_ = w.session.WriteClientResponseBodyChunk(data)
	}
	return w.ResponseWriter.Write(data)
}

func (w *clientResponseWriterWrapper) WriteString(s string) (int, error) {
	w.ensureHeadersWritten()
	if w.session != nil && len(s) > 0 {
		_ = w.session.WriteClientResponseBodyChunk([]byte(s))
	}
	return w.ResponseWriter.WriteString(s)
}

func (w *clientResponseWriterWrapper) ensureHeadersWritten() {
	if !w.headersWritten {
		statusCode := w.Status()
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		w.ensureHeadersWrittenWithStatus(statusCode)
	}
}

func (w *clientResponseWriterWrapper) ensureHeadersWrittenWithStatus(statusCode int) {
	if !w.headersWritten {
		w.headersWritten = true
		if w.session != nil {
			_ = w.session.WriteClientResponseHeaders(statusCode, w.proto, w.Header())
		}
	}
}
