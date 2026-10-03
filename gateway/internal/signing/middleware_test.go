package signing

import (
	"bytes"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVerify(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 3, 8, 0, 0, 0, time.UTC)
	middleware := New("secret", 5*time.Minute)
	middleware.now = func() time.Time { return now }
	handler := middleware.Verify(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := new(bytes.Buffer)
		_, _ = body.ReadFrom(r.Body)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write(body.Bytes())
	}))

	body := []byte(`{"command_id":"abc"}`)
	timestamp := now.Format(time.RFC3339)
	request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	request.Header.Set(TimestampHeader, timestamp)
	request.Header.Set(SignatureHeader, hex.EncodeToString(signature([]byte("secret"), timestamp, body)))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted || !bytes.Equal(response.Body.Bytes(), body) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestVerifyRejectsStaleAndInvalidSignatures(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 3, 8, 0, 0, 0, time.UTC)
	middleware := New("secret", 5*time.Minute)
	middleware.now = func() time.Time { return now }
	handler := middleware.Verify(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler should not run")
	}))

	tests := []struct {
		name      string
		timestamp string
		signature string
	}{
		{name: "stale", timestamp: now.Add(-6 * time.Minute).Format(time.RFC3339), signature: "00"},
		{name: "bad signature", timestamp: now.Format(time.RFC3339), signature: "00"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{}`)))
			request.Header.Set(TimestampHeader, test.timestamp)
			request.Header.Set(SignatureHeader, test.signature)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d", response.Code)
			}
		})
	}
}
