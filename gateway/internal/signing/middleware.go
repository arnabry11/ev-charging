package signing

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"time"
)

const (
	TimestampHeader = "X-Timestamp"
	SignatureHeader = "X-Signature"
	maxBodyBytes    = 1 << 20
)

type Middleware struct {
	secret []byte
	window time.Duration
	now    func() time.Time
}

func New(secret string, window time.Duration) *Middleware {
	return &Middleware{
		secret: []byte(secret),
		window: window,
		now:    time.Now,
	}
}

func (m *Middleware) Verify(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		timestamp := r.Header.Get(TimestampHeader)
		if !m.validTimestamp(timestamp) {
			http.Error(w, `{"error":"stale_or_invalid_timestamp"}`, http.StatusUnauthorized)
			return
		}

		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
		if err != nil {
			http.Error(w, `{"error":"invalid_body"}`, http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		if !validSignature(m.secret, timestamp, body, r.Header.Get(SignatureHeader)) {
			http.Error(w, `{"error":"invalid_signature"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) validTimestamp(value string) bool {
	sentAt, err := time.Parse(time.RFC3339, value)
	return err == nil && !outsideWindow(sentAt, m.now(), m.window)
}

func validSignature(secret []byte, timestamp string, body []byte, encoded string) bool {
	provided, err := hex.DecodeString(encoded)
	return err == nil && hmac.Equal(provided, signature(secret, timestamp, body))
}

func signature(secret []byte, timestamp string, body []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return mac.Sum(nil)
}

func Sign(secret []byte, timestamp string, body []byte) string {
	return hex.EncodeToString(signature(secret, timestamp, body))
}

func outsideWindow(sentAt, now time.Time, window time.Duration) bool {
	delta := now.Sub(sentAt)
	return delta < -window || delta > window
}
