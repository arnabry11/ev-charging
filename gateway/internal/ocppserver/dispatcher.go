package ocppserver

import (
	"context"
	"sync"
)

type commandTask struct {
	ctx    context.Context
	run    func() (bool, error)
	result chan commandResult
}

type commandDispatcher struct {
	mu      sync.Mutex
	workers map[string]chan commandTask
}

func newCommandDispatcher() *commandDispatcher {
	return &commandDispatcher{workers: make(map[string]chan commandTask)}
}

func (d *commandDispatcher) Do(ctx context.Context, chargerID string, run func() (bool, error)) (bool, error) {
	task := commandTask{
		ctx:    ctx,
		run:    run,
		result: make(chan commandResult, 1),
	}
	select {
	case d.worker(chargerID) <- task:
	case <-ctx.Done():
		return false, ctx.Err()
	}

	result := <-task.result
	return result.accepted, result.err
}

func (d *commandDispatcher) worker(chargerID string) chan commandTask {
	d.mu.Lock()
	defer d.mu.Unlock()

	if worker, ok := d.workers[chargerID]; ok {
		return worker
	}
	worker := make(chan commandTask)
	d.workers[chargerID] = worker
	go runCommandWorker(worker)
	return worker
}

func runCommandWorker(tasks <-chan commandTask) {
	for task := range tasks {
		if err := task.ctx.Err(); err != nil {
			task.result <- commandResult{err: err}
			continue
		}
		accepted, err := task.run()
		task.result <- commandResult{accepted: accepted, err: err}
	}
}
