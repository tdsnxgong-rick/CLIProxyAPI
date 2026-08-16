package trafficdump

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/logging"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/util"
)

// SessionContextKey is the key used to store the traffic dump session in context.
const SessionContextKey = "__traffic_dump_session__"

type sessionContextKeyType struct{}

var contextKey = sessionContextKeyType{}

// Session manages the dump files for a single request/response lifecycle.
type Session struct {
	mu               sync.Mutex
	sessionDir       string
	rawToken         bool
	closed           bool
	upstreamAttempts int
	clientRespF      *os.File
	upstreamResp     map[int]*os.File
	upHeadersOut     map[int]bool
}

// NewSession creates an isolated folder for the given request cycle and initializes the Session.
func NewSession(baseDir string, requestID string, timestamp time.Time, rawToken bool) (*Session, error) {
	if baseDir == "" {
		baseDir = "logs/traffic"
	}
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	sanitizedID := sanitizeID(requestID)
	if sanitizedID == "" {
		sanitizedID = logging.GenerateRequestID()
	}

	dateFolder := timestamp.Format("2006-01-02")
	timeFolder := fmt.Sprintf("%s-%03d-%s", timestamp.Format("15-04-05"), timestamp.Nanosecond()/1e6, sanitizedID)
	sessionDir := filepath.Join(baseDir, dateFolder, timeFolder)

	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return nil, fmt.Errorf("create traffic dump directory %s: %w", sessionDir, err)
	}

	return &Session{
		sessionDir:   sessionDir,
		rawToken:     rawToken,
		upstreamResp: make(map[int]*os.File),
		upHeadersOut: make(map[int]bool),
	}, nil
}

// WithContext returns a new context with the session attached.
func WithContext(ctx context.Context, session *Session) context.Context {
	return context.WithValue(ctx, contextKey, session)
}

// FromContext extracts the traffic dump session from a standard context or Gin context.
func FromContext(ctx context.Context) *Session {
	if ctx == nil {
		return nil
	}
	if session, ok := ctx.Value(contextKey).(*Session); ok && session != nil {
		return session
	}
	if ginCtx, ok := ctx.Value("gin").(*gin.Context); ok && ginCtx != nil {
		if val, exists := ginCtx.Get(SessionContextKey); exists {
			if session, ok := val.(*Session); ok {
				return session
			}
		}
	}
	return nil
}

// SetGinSession stores the traffic dump session in a Gin context.
func SetGinSession(c *gin.Context, session *Session) {
	if c != nil && session != nil {
		c.Set(SessionContextKey, session)
	}
}

// GetSessionDir returns the directory path for this session.
func (s *Session) GetSessionDir() string {
	if s == nil {
		return ""
	}
	return s.sessionDir
}

// WriteClientRequest writes client-request.txt in HTTP wire format.
func (s *Session) WriteClientRequest(method, uri, proto string, headers http.Header, body []byte) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}

	filePath := filepath.Join(s.sessionDir, "client-request.txt")
	content := s.formatHTTPRequest(method, uri, proto, headers, body)
	return os.WriteFile(filePath, content, 0644)
}

// WriteClientResponseHeaders writes the status line and headers to client-response.txt.
func (s *Session) WriteClientResponseHeaders(statusCode int, proto string, headers http.Header) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}

	f, err := s.ensureClientResponseFile()
	if err != nil {
		return err
	}

	head := s.formatResponseHead(proto, statusCode, headers)
	_, errWrite := f.WriteString(head)
	return errWrite
}

// WriteClientResponseBodyChunk writes a response body chunk to client-response.txt.
func (s *Session) WriteClientResponseBodyChunk(chunk []byte) error {
	if s == nil || len(chunk) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}

	f, err := s.ensureClientResponseFile()
	if err != nil {
		return err
	}

	_, errWrite := f.Write(chunk)
	return errWrite
}

// WriteUpstreamRequest writes upstream-request.txt (or upstream-request-N.txt) in HTTP wire format.
func (s *Session) WriteUpstreamRequest(attempt int, method, uri, proto string, headers http.Header, body []byte) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}

	if attempt <= 0 {
		s.upstreamAttempts++
		attempt = s.upstreamAttempts
	} else if attempt > s.upstreamAttempts {
		s.upstreamAttempts = attempt
	}

	filename := "upstream-request.txt"
	if attempt > 1 {
		filename = fmt.Sprintf("upstream-request-%d.txt", attempt)
	}
	filePath := filepath.Join(s.sessionDir, filename)

	content := s.formatHTTPRequest(method, uri, proto, headers, body)
	return os.WriteFile(filePath, content, 0644)
}

// WriteUpstreamResponseHeaders writes status line and headers for an upstream attempt.
func (s *Session) WriteUpstreamResponseHeaders(attempt int, statusCode int, proto string, headers http.Header) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}

	attempt = s.resolveAttempt(attempt)

	f, err := s.ensureUpstreamResponseFile(attempt)
	if err != nil {
		return err
	}

	head := s.formatResponseHead(proto, statusCode, headers)

	s.upHeadersOut[attempt] = true
	_, errWrite := f.WriteString(head)
	return errWrite
}

// WriteUpstreamResponseBodyChunk appends chunk data for an upstream attempt.
func (s *Session) WriteUpstreamResponseBodyChunk(attempt int, chunk []byte) error {
	if s == nil || len(chunk) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}

	attempt = s.resolveAttempt(attempt)

	f, err := s.ensureUpstreamResponseFile(attempt)
	if err != nil {
		return err
	}

	_, errWrite := f.Write(chunk)
	return errWrite
}

// WriteUpstreamError records an upstream failure or network error.
func (s *Session) WriteUpstreamError(attempt int, err error) error {
	if s == nil || err == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}

	attempt = s.resolveAttempt(attempt)

	f, errFile := s.ensureUpstreamResponseFile(attempt)
	if errFile != nil {
		return errFile
	}

	var sb strings.Builder
	if !s.upHeadersOut[attempt] {
		sb.WriteString("HTTP/1.1 502 Bad Gateway\n")
		sb.WriteString(fmt.Sprintf("X-Proxy-Error: %s\n\n", sanitizeHeaderValue(err.Error())))
		s.upHeadersOut[attempt] = true
	} else {
		sb.WriteString("\n")
	}
	sb.WriteString(fmt.Sprintf("[UPSTREAM ERROR: %s]\n", err.Error()))

	_, errWrite := f.WriteString(sb.String())
	return errWrite
}

// Close flushes and closes all open file handles in the session.
func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true

	var lastErr error
	if s.clientRespF != nil {
		if err := s.clientRespF.Close(); err != nil {
			lastErr = err
		}
		s.clientRespF = nil
	}
	for attempt, f := range s.upstreamResp {
		if f != nil {
			if err := f.Close(); err != nil {
				lastErr = err
			}
		}
		delete(s.upstreamResp, attempt)
	}
	return lastErr
}

func (s *Session) ensureClientResponseFile() (*os.File, error) {
	if s.clientRespF != nil {
		return s.clientRespF, nil
	}
	filePath := filepath.Join(s.sessionDir, "client-response.txt")
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open client-response.txt: %w", err)
	}
	s.clientRespF = f
	return f, nil
}

func (s *Session) ensureUpstreamResponseFile(attempt int) (*os.File, error) {
	if attempt <= 0 {
		attempt = 1
	}
	if f, ok := s.upstreamResp[attempt]; ok && f != nil {
		return f, nil
	}

	filename := "upstream-response.txt"
	if attempt > 1 {
		filename = fmt.Sprintf("upstream-response-%d.txt", attempt)
	}
	filePath := filepath.Join(s.sessionDir, filename)
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", filename, err)
	}
	s.upstreamResp[attempt] = f
	return f, nil
}

func (s *Session) formatHTTPRequest(method, uri, proto string, headers http.Header, body []byte) []byte {
	if proto == "" {
		proto = "HTTP/1.1"
	}
	if uri == "" {
		uri = "/"
	}
	if method == "" {
		method = "GET"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s %s %s\n", method, uri, proto))
	s.writeHeaderLines(&sb, headers)
	sb.WriteString("\n")

	headerBytes := []byte(sb.String())
	if len(body) == 0 {
		return headerBytes
	}

	res := make([]byte, len(headerBytes)+len(body))
	copy(res, headerBytes)
	copy(res[len(headerBytes):], body)
	return res
}

func (s *Session) resolveAttempt(attempt int) int {
	if attempt <= 0 {
		attempt = s.upstreamAttempts
		if attempt <= 0 {
			attempt = 1
		}
	}
	return attempt
}

func (s *Session) formatResponseHead(proto string, statusCode int, headers http.Header) string {
	statusText := http.StatusText(statusCode)
	if statusText == "" {
		statusText = "Unknown"
	}
	if proto == "" {
		proto = "HTTP/1.1"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s %d %s\n", proto, statusCode, statusText))
	s.writeHeaderLines(&sb, headers)
	sb.WriteString("\n")
	return sb.String()
}

func (s *Session) writeHeaderLines(sb *strings.Builder, headers http.Header) {
	if headers == nil {
		return
	}

	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		values := headers[k]
		for _, v := range values {
			outVal := v
			if !s.rawToken {
				outVal = util.MaskSensitiveHeaderValue(k, v)
			}
			sb.WriteString(fmt.Sprintf("%s: %s\n", k, outVal))
		}
	}
}

func sanitizeID(id string) string {
	id = strings.TrimSpace(id)
	reg := regexp.MustCompile(`[^a-zA-Z0-9_\-\.]`)
	sanitized := reg.ReplaceAllString(id, "_")
	return strings.Trim(sanitized, "_")
}

func sanitizeHeaderValue(val string) string {
	val = strings.ReplaceAll(val, "\r", " ")
	val = strings.ReplaceAll(val, "\n", " ")
	return strings.TrimSpace(val)
}
