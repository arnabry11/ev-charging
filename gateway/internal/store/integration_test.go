package store_test

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arnabry11/ev-charging/gateway/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCommandInsertAndClaimAreConcurrentSafe(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "TRUNCATE sessions, command_inbox"); err != nil {
		t.Fatal(err)
	}

	queries := store.New(pool)
	commandID := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	params := store.InsertCommandParams{
		CommandID:   commandID,
		CommandType: "start_session",
		Request:     []byte(`{"session_ref":"one"}`),
	}

	var inserts atomic.Int32
	runConcurrently(t, 20, func() error {
		count, err := queries.InsertCommand(ctx, params)
		inserts.Add(int32(count))
		return err
	})
	if got := inserts.Load(); got != 1 {
		t.Fatalf("insert winners = %d, want 1", got)
	}

	var claims atomic.Int32
	runConcurrently(t, 20, func() error {
		_, err := queries.ClaimCommand(ctx, store.ClaimCommandParams{
			CommandID:  commandID,
			ClaimToken: pgtype.UUID{Bytes: uuid.New(), Valid: true},
		})
		if err == nil {
			claims.Add(1)
			return nil
		}
		if err == pgx.ErrNoRows {
			return nil
		}
		return err
	})
	if got := claims.Load(); got != 1 {
		t.Fatalf("claim winners = %d, want 1", got)
	}

	firstClaim, err := queries.GetCommand(ctx, commandID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE command_inbox SET locked_until = now() - interval '1 second' WHERE command_id = $1", commandID); err != nil {
		t.Fatal(err)
	}
	secondToken := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	if _, err := queries.ClaimCommand(ctx, store.ClaimCommandParams{
		CommandID:  commandID,
		ClaimToken: secondToken,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.MarkCommandDispatching(ctx, store.MarkCommandDispatchingParams{
		CommandID:  commandID,
		ClaimToken: firstClaim.ClaimToken,
	}); err != pgx.ErrNoRows {
		t.Fatalf("stale dispatch error = %v, want no rows", err)
	}
	if _, err := queries.MarkCommandDispatching(ctx, store.MarkCommandDispatchingParams{
		CommandID:  commandID,
		ClaimToken: secondToken,
	}); err != nil {
		t.Fatal(err)
	}
	staleCompletions, err := queries.CompleteCommand(ctx, store.CompleteCommandParams{
		CommandID:  commandID,
		Result:     []byte(`{"state":"failed"}`),
		HttpStatus: pgtype.Int4{Int32: 202, Valid: true},
		ClaimToken: firstClaim.ClaimToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	if staleCompletions != 0 {
		t.Fatalf("stale completions = %d, want 0", staleCompletions)
	}
	currentCompletions, err := queries.CompleteCommand(ctx, store.CompleteCommandParams{
		CommandID:  commandID,
		Result:     []byte(`{"state":"start_requested","result":"accepted"}`),
		HttpStatus: pgtype.Int4{Int32: 202, Valid: true},
		ClaimToken: secondToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	if currentCompletions != 1 {
		t.Fatalf("current completions = %d, want 1", currentCompletions)
	}
}

func TestOutboxSequencesAreUniqueUnderConcurrency(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "TRUNCATE outbox, session_event_sequences"); err != nil {
		t.Fatal(err)
	}

	queries := store.New(pool)
	sessionRef := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	sequences := make(chan int64, 20)
	runConcurrently(t, 20, func() error {
		event, err := queries.AppendOutboxEvent(ctx, store.AppendOutboxEventParams{
			SessionRef: sessionRef,
			EventType:  "session.meter_values",
			Payload:    []byte(`{"energy_wh":1}`),
			OccurredAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		})
		if err == nil {
			sequences <- event.Sequence
		}
		return err
	})
	close(sequences)

	seen := map[int64]bool{}
	for sequence := range sequences {
		if sequence < 1 || sequence > 20 || seen[sequence] {
			t.Fatalf("duplicate or invalid sequence %d", sequence)
		}
		seen[sequence] = true
	}
	if len(seen) != 20 {
		t.Fatalf("sequences = %d, want 20", len(seen))
	}
}

func TestOutboxClaimPublishesEachSessionInOrder(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "TRUNCATE outbox, session_event_sequences"); err != nil {
		t.Fatal(err)
	}

	queries := store.New(pool)
	sessionRef := pgtype.UUID{Bytes: uuid.New(), Valid: true}
	for range 2 {
		if _, err := queries.AppendOutboxEvent(ctx, store.AppendOutboxEventParams{
			SessionRef: sessionRef,
			EventType:  "session.meter_values",
			Payload:    []byte(`{"energy_wh":1}`),
			OccurredAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		}); err != nil {
			t.Fatal(err)
		}
	}

	first, err := queries.ClaimOutboxBatch(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].Sequence != 1 {
		t.Fatalf("first claim = %+v", first)
	}
	second, err := queries.ClaimOutboxBatch(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 0 {
		t.Fatalf("second claim = %+v, want none while sequence 1 is unpublished", second)
	}
	if _, err := queries.MarkOutboxPublished(ctx, first[0].EventID); err != nil {
		t.Fatal(err)
	}
	next, err := queries.ClaimOutboxBatch(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(next) != 1 || next[0].Sequence != 2 {
		t.Fatalf("next claim = %+v", next)
	}
}

func outboxTestPool(t *testing.T) (*pgxpool.Pool, *store.Queries) {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "TRUNCATE outbox, session_event_sequences"); err != nil {
		t.Fatal(err)
	}
	return pool, store.New(pool)
}

func appendOutboxEvents(t *testing.T, queries *store.Queries, sessionRef pgtype.UUID, count int) {
	t.Helper()

	for range count {
		if _, err := queries.AppendOutboxEvent(context.Background(), store.AppendOutboxEventParams{
			SessionRef: sessionRef,
			EventType:  "session.meter_values",
			Payload:    []byte(`{"energy_wh":1}`),
			OccurredAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func newSessionRef() pgtype.UUID {
	return pgtype.UUID{Bytes: uuid.New(), Valid: true}
}

func secondsUntilRetry(t *testing.T, pool *pgxpool.Pool, eventID pgtype.UUID) float64 {
	t.Helper()

	var seconds float64
	if err := pool.QueryRow(context.Background(),
		"SELECT extract(epoch FROM (locked_until - now()))::float8 FROM outbox WHERE event_id = $1", eventID).Scan(&seconds); err != nil {
		t.Fatal(err)
	}
	return seconds
}

func TestOutboxRefusalsAreCountedAndTheEventIsDeadLetteredAtTheLimit(t *testing.T) {
	ctx := context.Background()
	pool, queries := outboxTestPool(t)
	appendOutboxEvents(t, queries, newSessionRef(), 1)

	var event store.Outbox
	for refusal := int32(1); refusal <= 3; refusal++ {
		if _, err := pool.Exec(ctx, "UPDATE outbox SET locked_until = NULL"); err != nil {
			t.Fatal(err)
		}
		claimed, err := queries.ClaimOutboxBatch(ctx, 10)
		if err != nil || len(claimed) != 1 {
			t.Fatalf("claim %d: %v %+v", refusal, err, claimed)
		}
		event = claimed[0]
		dead, err := queries.RejectOutbox(ctx, store.RejectOutboxParams{EventID: event.EventID, LastError: "platform returned 409: sequence_gap", MaxRejections: 3})
		if err != nil {
			t.Fatal(err)
		}
		if want := refusal == 3; dead != want {
			t.Fatalf("refusal %d: dead = %v, want %v", refusal, dead, want)
		}
	}

	var rejections int32
	var lastError string
	var deadAt pgtype.Timestamptz
	if err := pool.QueryRow(ctx, "SELECT rejections, last_error, dead_at FROM outbox WHERE event_id = $1", event.EventID).Scan(&rejections, &lastError, &deadAt); err != nil {
		t.Fatal(err)
	}
	if rejections != 3 || lastError != "platform returned 409: sequence_gap" || !deadAt.Valid {
		t.Fatalf("rejections=%d last_error=%q dead_at=%v", rejections, lastError, deadAt)
	}

	if _, err := pool.Exec(ctx, "UPDATE outbox SET locked_until = NULL"); err != nil {
		t.Fatal(err)
	}
	again, err := queries.ClaimOutboxBatch(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("a dead event was claimed again: %+v", again)
	}
}

func TestOutboxRetryDelayDoublesUpToAMinuteAndFailuresAreNotRefusals(t *testing.T) {
	ctx := context.Background()
	pool, queries := outboxTestPool(t)
	appendOutboxEvents(t, queries, newSessionRef(), 1)
	claimed, err := queries.ClaimOutboxBatch(ctx, 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: %v %+v", err, claimed)
	}
	eventID := claimed[0].EventID

	for attempts, want := range map[int]float64{1: 1, 2: 2, 3: 4, 4: 8, 5: 16, 6: 32, 7: 60, 8: 60, 40: 60} {
		if _, err := pool.Exec(ctx, "UPDATE outbox SET attempts = $2 WHERE event_id = $1", eventID, attempts); err != nil {
			t.Fatal(err)
		}
		if err := queries.ReleaseOutbox(ctx, store.ReleaseOutboxParams{EventID: eventID, LastError: "platform returned 503"}); err != nil {
			t.Fatal(err)
		}
		if got := secondsUntilRetry(t, pool, eventID); got > want || got < want-2 {
			t.Fatalf("attempt %d: retry in %.1fs, want about %.0fs", attempts, got, want)
		}
	}

	var rejections int32
	var lastError string
	var deadAt pgtype.Timestamptz
	if err := pool.QueryRow(ctx, "SELECT rejections, last_error, dead_at FROM outbox WHERE event_id = $1", eventID).Scan(&rejections, &lastError, &deadAt); err != nil {
		t.Fatal(err)
	}
	if rejections != 0 || deadAt.Valid || lastError != "platform returned 503" {
		t.Fatalf("a failed delivery counted as a refusal: rejections=%d dead_at=%v last_error=%q", rejections, deadAt, lastError)
	}
}

func TestOutboxRejectionBackoffMatchesTheRetryDelay(t *testing.T) {
	ctx := context.Background()
	pool, queries := outboxTestPool(t)
	appendOutboxEvents(t, queries, newSessionRef(), 1)
	claimed, err := queries.ClaimOutboxBatch(ctx, 10)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: %v %+v", err, claimed)
	}
	if _, err := pool.Exec(ctx, "UPDATE outbox SET attempts = 5"); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.RejectOutbox(ctx, store.RejectOutboxParams{EventID: claimed[0].EventID, LastError: "x", MaxRejections: 10}); err != nil {
		t.Fatal(err)
	}
	if got := secondsUntilRetry(t, pool, claimed[0].EventID); got > 16 || got < 14 {
		t.Fatalf("retry in %.1fs, want about 16s", got)
	}
}

func TestOutboxSweepDeadLettersOnlyTheEventsBehindADeadEvent(t *testing.T) {
	ctx := context.Background()
	pool, queries := outboxTestPool(t)
	stuck, healthy := newSessionRef(), newSessionRef()
	appendOutboxEvents(t, queries, stuck, 4)
	appendOutboxEvents(t, queries, healthy, 2)

	// Sequence 1 of the stuck session was published; sequence 2 is dead.
	if _, err := pool.Exec(ctx, "UPDATE outbox SET published_at = now() WHERE session_ref = $1 AND sequence = 1", stuck); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE outbox SET dead_at = now(), last_error = 'refused' WHERE session_ref = $1 AND sequence = 2", stuck); err != nil {
		t.Fatal(err)
	}

	swept, err := queries.DeadLetterBlockedOutbox(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if swept != 2 {
		t.Fatalf("swept = %d, want the 2 events behind the dead one", swept)
	}
	again, err := queries.DeadLetterBlockedOutbox(ctx)
	if err != nil || again != 0 {
		t.Fatalf("a second sweep changed %d rows (err %v)", again, err)
	}

	rows, err := pool.Query(ctx, "SELECT session_ref, sequence, dead_at IS NOT NULL, coalesce(last_error, '') FROM outbox ORDER BY session_ref, sequence")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	deadBySession := map[pgtype.UUID][]int64{}
	for rows.Next() {
		var ref pgtype.UUID
		var sequence int64
		var dead bool
		var lastError string
		if err := rows.Scan(&ref, &sequence, &dead, &lastError); err != nil {
			t.Fatal(err)
		}
		if dead {
			deadBySession[ref] = append(deadBySession[ref], sequence)
		}
		if ref == stuck && sequence >= 3 && lastError != "behind a dead event in the same session" {
			t.Fatalf("sequence %d last_error = %q", sequence, lastError)
		}
	}
	if got := deadBySession[stuck]; len(got) != 3 || got[0] != 2 || got[1] != 3 || got[2] != 4 {
		t.Fatalf("dead events in the stuck session = %v, want [2 3 4]", got)
	}
	if len(deadBySession[healthy]) != 0 {
		t.Fatalf("the healthy session was dead-lettered: %v", deadBySession[healthy])
	}
}

func TestOutboxClaimKeepsServingHealthySessionsWhileAnotherIsDead(t *testing.T) {
	ctx := context.Background()
	pool, queries := outboxTestPool(t)
	stuck, healthy := newSessionRef(), newSessionRef()
	appendOutboxEvents(t, queries, stuck, 3)
	appendOutboxEvents(t, queries, healthy, 1)
	if _, err := pool.Exec(ctx, "UPDATE outbox SET dead_at = now() WHERE session_ref = $1 AND sequence = 1", stuck); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.DeadLetterBlockedOutbox(ctx); err != nil {
		t.Fatal(err)
	}

	claimed, err := queries.ClaimOutboxBatch(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0].SessionRef != healthy {
		t.Fatalf("claimed = %+v, want only the healthy session's event", claimed)
	}
}

func TestOutboxDeadEventsCanBeRequeuedWithOneStatement(t *testing.T) {
	ctx := context.Background()
	pool, queries := outboxTestPool(t)
	stuck := newSessionRef()
	appendOutboxEvents(t, queries, stuck, 3)
	if _, err := pool.Exec(ctx, "UPDATE outbox SET dead_at = now(), rejections = 10, last_error = 'refused' WHERE sequence = 1"); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.DeadLetterBlockedOutbox(ctx); err != nil {
		t.Fatal(err)
	}

	// The statement documented in the gateway README.
	tag, err := pool.Exec(ctx, "UPDATE outbox SET dead_at = NULL, rejections = 0, last_error = NULL, locked_until = NULL WHERE dead_at IS NOT NULL")
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 3 {
		t.Fatalf("requeued %d events, want 3", tag.RowsAffected())
	}

	for sequence := int64(1); sequence <= 3; sequence++ {
		claimed, err := queries.ClaimOutboxBatch(ctx, 10)
		if err != nil || len(claimed) != 1 || claimed[0].Sequence != sequence {
			t.Fatalf("claim for sequence %d: %v %+v", sequence, err, claimed)
		}
		if _, err := queries.MarkOutboxPublished(ctx, claimed[0].EventID); err != nil {
			t.Fatal(err)
		}
	}
}

func runConcurrently(t *testing.T, count int, operation func() error) {
	t.Helper()

	start := make(chan struct{})
	errs := make(chan error, count)
	var group sync.WaitGroup
	for range count {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			errs <- operation()
		}()
	}
	close(start)
	group.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}
