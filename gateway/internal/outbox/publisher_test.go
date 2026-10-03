package outbox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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
	if len(repo.released) != 0 || len(repo.rejected) != 0 {
		t.Fatalf("released = %+v rejected = %+v", repo.released, repo.rejected)
	}
}

func TestPublisherReleasesEventAfterPlatformFailure(t *testing.T) {
	t.Parallel()

	event := sampleEvent()
	repo := &fakeOutbox{events: []store.Outbox{event}}
	server := statusServer(t, http.StatusServiceUnavailable)

	publisher := New(repo, server.URL, "secret", quietLogger())
	if err := publisher.Publish(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repo.published) != 0 || len(repo.released) != 1 || len(repo.rejected) != 0 {
		t.Fatalf("published=%d released=%d rejected=%d", len(repo.published), len(repo.released), len(repo.rejected))
	}
	if !strings.Contains(repo.released[0].LastError, "503") {
		t.Fatalf("last error = %q", repo.released[0].LastError)
	}
}

// A platform that is down, slow or misconfigured says nothing about the event, so
// those failures are retried for as long as it takes and never count towards giving up.
func TestPublisherNeverCountsOutagesAndConfigurationErrorsAsRejections(t *testing.T) {
	t.Parallel()

	for _, status := range []int{
		http.StatusInternalServerError, http.StatusBadGateway, http.StatusGatewayTimeout,
		http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound,
		http.StatusRequestTimeout, http.StatusTooManyRequests,
	} {
		repo := &fakeOutbox{events: []store.Outbox{sampleEvent()}}
		publisher := New(repo, statusServer(t, status).URL, "secret", quietLogger())
		if err := publisher.Publish(context.Background()); err != nil {
			t.Fatal(err)
		}
		if len(repo.released) != 1 || len(repo.rejected) != 0 {
			t.Fatalf("status %d: released=%d rejected=%d", status, len(repo.released), len(repo.rejected))
		}
	}
}

func TestPublisherReleasesEventWhenThePlatformIsUnreachable(t *testing.T) {
	t.Parallel()

	server := statusServer(t, http.StatusOK)
	url := server.URL
	server.Close()

	repo := &fakeOutbox{events: []store.Outbox{sampleEvent()}}
	publisher := New(repo, url, "secret", quietLogger())
	if err := publisher.Publish(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repo.released) != 1 || len(repo.rejected) != 0 {
		t.Fatalf("released=%d rejected=%d", len(repo.released), len(repo.rejected))
	}
}

func TestPublisherCountsARefusedEventAsARejection(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusBadRequest, http.StatusConflict, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity} {
		event := sampleEvent()
		repo := &fakeOutbox{events: []store.Outbox{event}}
		publisher := New(repo, statusServer(t, status).URL, "secret", quietLogger())
		publisher.SetMaxRejections(7)
		if err := publisher.Publish(context.Background()); err != nil {
			t.Fatal(err)
		}
		if len(repo.released) != 0 || len(repo.rejected) != 1 {
			t.Fatalf("status %d: released=%d rejected=%d", status, len(repo.released), len(repo.rejected))
		}
		got := repo.rejected[0]
		if got.EventID != event.EventID || got.MaxRejections != 7 || !strings.Contains(got.LastError, fmt.Sprintf("platform returned %d", status)) {
			t.Fatalf("status %d: rejection = %+v", status, got)
		}
	}
}

func TestPublisherRejectionLimitDefaultsToTen(t *testing.T) {
	t.Parallel()

	repo := &fakeOutbox{events: []store.Outbox{sampleEvent()}}
	publisher := New(repo, statusServer(t, http.StatusConflict).URL, "secret", quietLogger())
	if err := publisher.Publish(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repo.rejected[0].MaxRejections != 10 {
		t.Fatalf("max rejections = %d", repo.rejected[0].MaxRejections)
	}
}

func TestPublisherLogsOnceWhenItGivesUp(t *testing.T) {
	t.Parallel()

	event := sampleEvent()
	repo := &fakeOutbox{events: []store.Outbox{event}, dead: true}
	var logs bytes.Buffer
	publisher := New(repo, statusServer(t, http.StatusConflict).URL, "secret", slog.New(slog.NewTextHandler(&logs, nil)))
	if err := publisher.Publish(context.Background()); err != nil {
		t.Fatal(err)
	}

	output := logs.String()
	for _, want := range []string{"gave up on gateway event", "level=ERROR", uuid.UUID(event.EventID.Bytes).String(), uuid.UUID(event.SessionRef.Bytes).String(), "sequence=1", "409"} {
		if !strings.Contains(output, want) {
			t.Fatalf("log missing %q:\n%s", want, output)
		}
	}
}

func TestPublisherDoesNotLogGivingUpForAnEventThatCanStillBeRetried(t *testing.T) {
	t.Parallel()

	repo := &fakeOutbox{events: []store.Outbox{sampleEvent()}, dead: false}
	var logs bytes.Buffer
	publisher := New(repo, statusServer(t, http.StatusConflict).URL, "secret", slog.New(slog.NewTextHandler(&logs, nil)))
	if err := publisher.Publish(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs.String(), "gave up") {
		t.Fatalf("unexpected give-up log:\n%s", logs.String())
	}
}

func TestPublisherSweepsEventsBehindADeadEventBeforeClaiming(t *testing.T) {
	t.Parallel()

	repo := &fakeOutbox{swept: 3}
	var logs bytes.Buffer
	publisher := New(repo, "http://unused.invalid", "secret", slog.New(slog.NewTextHandler(&logs, nil)))
	if err := publisher.Publish(context.Background()); err != nil {
		t.Fatal(err)
	}

	if repo.order[0] != "sweep" || repo.order[1] != "claim" {
		t.Fatalf("call order = %v, want the sweep before the claim", repo.order)
	}
	if !strings.Contains(logs.String(), "count=3") {
		t.Fatalf("sweep was not logged:\n%s", logs.String())
	}
}

func statusServer(t *testing.T, status int) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, http.StatusText(status), status)
	}))
	t.Cleanup(server.Close)
	return server
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
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
	released  []store.ReleaseOutboxParams
	rejected  []store.RejectOutboxParams
	dead      bool
	swept     int64
	order     []string
}

func (f *fakeOutbox) ClaimOutboxBatch(context.Context, int32) ([]store.Outbox, error) {
	f.order = append(f.order, "claim")
	events := f.events
	f.events = nil
	return events, nil
}

func (f *fakeOutbox) MarkOutboxPublished(_ context.Context, id pgtype.UUID) (int64, error) {
	f.published = append(f.published, id)
	return 1, nil
}

func (f *fakeOutbox) ReleaseOutbox(_ context.Context, arg store.ReleaseOutboxParams) error {
	f.released = append(f.released, arg)
	return nil
}

func (f *fakeOutbox) RejectOutbox(_ context.Context, arg store.RejectOutboxParams) (bool, error) {
	f.rejected = append(f.rejected, arg)
	return f.dead, nil
}

func (f *fakeOutbox) DeadLetterBlockedOutbox(context.Context) (int64, error) {
	f.order = append(f.order, "sweep")
	return f.swept, nil
}
