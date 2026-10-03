package outbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/arnabry11/ev-charging/gateway/internal/signing"
	"github.com/arnabry11/ev-charging/gateway/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// defaultMaxRejections is how many times the platform may refuse one event
// before the gateway gives up on it. With the retry delay doubling up to a
// minute, that is about five minutes.
const defaultMaxRejections = 10

type repository interface {
	ClaimOutboxBatch(context.Context, int32) ([]store.Outbox, error)
	MarkOutboxPublished(context.Context, pgtype.UUID) (int64, error)
	ReleaseOutbox(context.Context, store.ReleaseOutboxParams) error
	RejectOutbox(context.Context, store.RejectOutboxParams) (bool, error)
	DeadLetterBlockedOutbox(context.Context) (int64, error)
}

type Publisher struct {
	repo          repository
	url           string
	secret        []byte
	client        *http.Client
	logger        *slog.Logger
	batchSize     int32
	interval      time.Duration
	maxRejections int32
	now           func() time.Time
}

func New(repo repository, url, secret string, logger *slog.Logger) *Publisher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Publisher{
		repo:      repo,
		url:       url,
		secret:    []byte(secret),
		client:    &http.Client{Timeout: 5 * time.Second},
		logger:    logger,
		batchSize: 20,
		interval:  200 * time.Millisecond,
		now:       time.Now,

		maxRejections: defaultMaxRejections,
	}
}

// SetMaxRejections changes how many refusals one event gets before it is
// dead-lettered. Values below 1 are ignored.
func (p *Publisher) SetMaxRejections(limit int32) {
	if limit >= 1 {
		p.maxRejections = limit
	}
}

func (p *Publisher) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		if err := p.Publish(ctx); err != nil {
			p.logger.Error("publish outbox", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (p *Publisher) Publish(ctx context.Context) error {
	// Dead-letter what sits behind a dead event first, so it is never claimed.
	swept, err := p.repo.DeadLetterBlockedOutbox(ctx)
	if err != nil {
		return err
	}
	if swept > 0 {
		p.logger.Error("dead-lettered gateway events behind a dead event", "count", swept)
	}

	events, err := p.repo.ClaimOutboxBatch(ctx, p.batchSize)
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := p.publishOne(ctx, event); err != nil {
			p.handleFailure(ctx, event, err)
			continue
		}
		if _, err := p.repo.MarkOutboxPublished(ctx, event.EventID); err != nil {
			return err
		}
	}
	return nil
}

// handleFailure decides between "try again later" and "the platform refused
// this event". Only a refusal counts towards giving up: an outage or a wrong
// signing secret says nothing about the event, and giving up on those would
// throw away the backlog.
func (p *Publisher) handleFailure(ctx context.Context, event store.Outbox, err error) {
	fields := []any{
		"event_id", uuid.UUID(event.EventID.Bytes).String(),
		"session_ref", uuid.UUID(event.SessionRef.Bytes).String(),
		"sequence", event.Sequence,
		"attempts", event.Attempts,
		"err", err,
	}

	var refused *refusedError
	if !errors.As(err, &refused) {
		if releaseErr := p.repo.ReleaseOutbox(ctx, store.ReleaseOutboxParams{EventID: event.EventID, LastError: err.Error()}); releaseErr != nil {
			p.logger.Error("release outbox event", "event_id", uuid.UUID(event.EventID.Bytes).String(), "err", releaseErr)
		}
		p.logger.Warn("gateway event delivery failed, will retry", fields...)
		return
	}

	dead, rejectErr := p.repo.RejectOutbox(ctx, store.RejectOutboxParams{
		EventID:       event.EventID,
		LastError:     err.Error(),
		MaxRejections: p.maxRejections,
	})
	if rejectErr != nil {
		p.logger.Error("record outbox rejection", "event_id", uuid.UUID(event.EventID.Bytes).String(), "err", rejectErr)
		return
	}
	if dead {
		p.logger.Error("gave up on gateway event", fields...)
		return
	}
	p.logger.Warn("platform refused gateway event, will retry", fields...)
}

func (p *Publisher) publishOne(ctx context.Context, event store.Outbox) error {
	body, err := json.Marshal(envelope{
		EventID:    uuid.UUID(event.EventID.Bytes).String(),
		EventType:  event.EventType,
		SessionRef: uuid.UUID(event.SessionRef.Bytes).String(),
		Sequence:   event.Sequence,
		OccurredAt: event.OccurredAt.Time.UTC().Format(time.RFC3339),
		Payload:    event.Payload,
	})
	if err != nil {
		return err
	}
	timestamp := p.now().UTC().Format(time.RFC3339)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(signing.TimestampHeader, timestamp)
	request.Header.Set(signing.SignatureHeader, signing.Sign(p.secret, timestamp, body))

	response, err := p.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 180))
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}
	failure := fmt.Errorf("platform returned %d: %s", response.StatusCode, responseBody)
	if refusesEvent(response.StatusCode) {
		return &refusedError{err: failure}
	}
	return failure
}

// refusesEvent reports whether a status means the platform looked at this event
// and said no (bad request, sequence gap, too large, invalid), as opposed to
// being unavailable or unhappy with our credentials.
func refusesEvent(status int) bool {
	switch status {
	case http.StatusBadRequest, http.StatusConflict, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity:
		return true
	}
	return false
}

type refusedError struct{ err error }

func (e *refusedError) Error() string { return e.err.Error() }
func (e *refusedError) Unwrap() error { return e.err }

type envelope struct {
	EventID    string          `json:"event_id"`
	EventType  string          `json:"event_type"`
	SessionRef string          `json:"session_ref"`
	Sequence   int64           `json:"sequence"`
	OccurredAt string          `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload"`
}
