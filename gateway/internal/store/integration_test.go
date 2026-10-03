package store_test

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"

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
