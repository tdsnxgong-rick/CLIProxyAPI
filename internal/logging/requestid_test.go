package logging

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractOrGenerateRequestID(t *testing.T) {
	// Case 1: Custom X-Request-ID header present
	reqWithHeader := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	reqWithHeader.Header.Set("X-Request-ID", "custom-client-req-12345")

	id := ExtractOrGenerateRequestID(reqWithHeader)
	if id != "custom-client-req-12345" {
		t.Errorf("expected %q, got %q", "custom-client-req-12345", id)
	}

	// Case 2: X-Request-ID header with leading/trailing whitespace
	reqWithSpaces := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	reqWithSpaces.Header.Set("X-Request-ID", "  custom-trimmed-id  ")

	idTrimmed := ExtractOrGenerateRequestID(reqWithSpaces)
	if idTrimmed != "custom-trimmed-id" {
		t.Errorf("expected %q, got %q", "custom-trimmed-id", idTrimmed)
	}

	// Case 3: Empty X-Request-ID header -> generate random 8 hex chars
	reqEmpty := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	reqEmpty.Header.Set("X-Request-ID", "   ")

	idGen := ExtractOrGenerateRequestID(reqEmpty)
	if len(idGen) != 8 {
		t.Errorf("expected 8-char hex string, got %q (len %d)", idGen, len(idGen))
	}

	// Case 4: No header -> generate random 8 hex chars
	reqNilHeader := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	idNilHeader := ExtractOrGenerateRequestID(reqNilHeader)
	if len(idNilHeader) != 8 {
		t.Errorf("expected 8-char hex string, got %q (len %d)", idNilHeader, len(idNilHeader))
	}

	// Case 5: nil request
	idNil := ExtractOrGenerateRequestID(nil)
	if len(idNil) != 8 {
		t.Errorf("expected 8-char hex string, got %q (len %d)", idNil, len(idNil))
	}
}
