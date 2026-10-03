package outbox

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arnabry11/ev-charging/gateway/internal/signing"
	"github.com/arnabry11/ev-charging/gateway/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestPublisherSignsEventAndMarksItPublished(t *testing.T) {
	t.Parallel()

	event := sampleEvent()
	repo := &fakeOutbox{events: []store.Outbox{event}}
	var timestamp, signature string
	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		timestamp = r.Header.Get(signing.TimestampHeader)
		signature = r.Header.Get(signing.SignatureHeader)
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	publisher := New(repo, server.URL, "secret", slog.New(slog.NewTextHandler(io.Discard, nil)))
	now := time.Date(2026, time.October, 3, 9, 0, 0, 0, time.UTC)
	publisher.now = func() time.Time { return now }
	if err := publisher.Publish(context.Background()); err != nil {
		t.Fatal(err)
	}

	if signature != signing.Sign([]byte("secret"), timestamp, body) {
		t.Fatalf("signature = %s", signature)
	}
	if len(repo.published) != 1 || repo.published[0] != event.EventID {
		t.Fatalf("published = %+v", repo.published)
	}
	if len(repo.released) != 0 {
		t.Fatalf("released = %+v", repo.released)
	}
}

func TestPublisherReleasesEventAfterPlatformFailure(t *testing.T) {
	t.Parallel()

	event := sampleEvent()
	repo := &fakeOutbox{events: []store.Outbox{event}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	publisher := New(repo, server.URL, "secret", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := publisher.Publish(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repo.published) != 0 || len(repo.released) != 1 {
		t.Fatalf("published=%d released=%d", len(repo.published), len(repo.released))
	}
}

func sampleEvent() store.Outbox {
	return store.Outbox{
		EventID:    pgtype.UUID{Bytes: uuid.New(), Valid: true},
		SessionRef: pgtype.UUID{Bytes: uuid.New(), Valid: true},
		Sequence:   1,
		EventType:  "session.started",
		Payload:    []byte(`{"meter_start_wh":100000}`),
		OccurredAt: pgtype.Timestamptz{Time: time.Date(2026, time.October, 3, 9, 0, 0, 0, time.UTC), Valid: true},
	}
}

type fakeOutbox struct {
	events    []store.Outbox
	published []pgtype.UUID
	released  []pgtype.UUID
}

func (f *fakeOutbox) ClaimOutboxBatch(context.Context, int32) ([]store.Outbox, error) {
	events := f.events
	f.events = nil
	return events, nil
}

func (f *fakeOutbox) MarkOutboxPublished(_ context.Context, id pgtype.UUID) (int64, error) {
	f.published = append(f.published, id)
	return 1, nil
}

func (f *fakeOutbox) ReleaseOutbox(_ context.Context, id pgtype.UUID) error {
	f.released = append(f.released, id)
	return nil
}
