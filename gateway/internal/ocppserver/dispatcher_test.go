package ocppserver

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestCommandDispatcherSerializesEachCharger(t *testing.T) {
	t.Parallel()

	dispatcher := newCommandDispatcher()
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})

	go func() {
		_, _ = dispatcher.Do(context.Background(), "CHG-1", func() (bool, error) {
			close(firstStarted)
			<-releaseFirst
			return true, nil
		})
	}()
	<-firstStarted

	go func() {
		_, _ = dispatcher.Do(context.Background(), "CHG-1", func() (bool, error) {
			close(secondStarted)
			return true, nil
		})
	}()

	select {
	case <-secondStarted:
		t.Fatal("second command started before first completed")
	case <-time.After(20 * time.Millisecond):
	}
	close(releaseFirst)
	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("second command did not start")
	}
}

func TestCommandDispatcherAllowsDifferentChargers(t *testing.T) {
	t.Parallel()

	dispatcher := newCommandDispatcher()
	release := make(chan struct{})
	var started atomic.Int32
	for _, chargerID := range []string{"CHG-1", "CHG-2"} {
		chargerID := chargerID
		go func() {
			_, _ = dispatcher.Do(context.Background(), chargerID, func() (bool, error) {
				started.Add(1)
				<-release
				return true, nil
			})
		}()
	}

	deadline := time.Now().Add(time.Second)
	for started.Load() != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	close(release)
	if got := started.Load(); got != 2 {
		t.Fatalf("started commands = %d, want 2", got)
	}
}

func TestCommandDispatcherKeepsOwnershipAfterDispatchCancellation(t *testing.T) {
	t.Parallel()

	dispatcher := newCommandDispatcher()
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})
	result := make(chan commandResult, 1)
	go func() {
		accepted, err := dispatcher.Do(ctx, "CHG-1", func() (bool, error) {
			close(started)
			<-release
			return true, nil
		})
		result <- commandResult{accepted: accepted, err: err}
	}()
	<-started
	cancel()

	select {
	case <-result:
		t.Fatal("dispatcher returned before the in-flight command completed")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	response := <-result
	if !response.accepted || response.err != nil {
		t.Fatalf("response = %+v", response)
	}
}

func TestCommandDispatcherSkipsCanceledQueuedCommand(t *testing.T) {
	t.Parallel()

	dispatcher := newCommandDispatcher()
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	go func() {
		_, _ = dispatcher.Do(context.Background(), "CHG-1", func() (bool, error) {
			close(firstStarted)
			<-releaseFirst
			return true, nil
		})
	}()
	<-firstStarted

	ctx, cancel := context.WithCancel(context.Background())
	var ran atomic.Bool
	result := make(chan error, 1)
	go func() {
		_, err := dispatcher.Do(ctx, "CHG-1", func() (bool, error) {
			ran.Store(true)
			return true, nil
		})
		result <- err
	}()
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
	close(releaseFirst)
	if ran.Load() {
		t.Fatal("canceled queued command ran")
	}
}
