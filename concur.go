// Package concur provides idiomatic Go utilities for running concurrent tasks
// with limited concurrency. It supports common concurrency patterns like "first success",
// "all settled", and batched processing with rate limiting.
//
// The package offers two usage styles:
//  1. Functional API - Simple one-off concurrent operations (recommended for most cases)
//  2. Runner API - Reusable worker pool for repeated operations
//
// Example (Functional API):
//
//	tasks := []func(context.Context) (string, error){
//	    func(ctx context.Context) (string, error) { return fetchFromDB(ctx) },
//	    func(ctx context.Context) (string, error) { return fetchFromCache(ctx) },
//	}
//	result := concur.FirstSuccess(ctx, 10, tasks)
//
// The package builds on top of pond for efficient worker pool management.
package concur

import (
	"context"
	"errors"
	"time"

	"github.com/alitto/pond/v2"
)

var (
	// ErrNoSuccess indicates no task completed successfully.
	ErrNoSuccess = errors.New("no task succeeded")
	// ErrNoFailure indicates no task failed.
	ErrNoFailure = errors.New("no task failed")
	// ErrEmptyTasks indicates the task list is empty.
	ErrEmptyTasks = errors.New("tasks list is empty")
	// ErrTaskFailed indicates one or more tasks failed (used by All).
	ErrTaskFailed = errors.New("one or more tasks failed")
)

// TaskResult holds the outcome of a single concurrent task.
// It includes the original task index for tracking and ordering results.
type TaskResult[R any] struct {
	Index  int   // Index of the task in the original task slice
	Result R     // Result value (only valid when Err == nil)
	Err    error // Error returned by the task (nil on success)
}

// Runner manages a worker pool for executing concurrent tasks with bounded concurrency.
// It is designed for scenarios where you need to reuse the same pool for multiple operations,
// or when you need fine-grained control over task lifecycle.
//
// Example:
//
//	runner := concur.NewRunner[string](ctx, 5)
//	defer runner.Stop()
//	result := runner.FirstSuccess(tasks)
//
// For one-off operations, prefer the functional API (FirstSuccess, All, etc.).
type Runner[R any] struct {
	pool   pond.Pool
	ctx    context.Context
	cancel context.CancelFunc
}

// NewRunner creates a new Runner with the specified concurrency limit.
// The concurrency parameter controls the maximum number of tasks that can run simultaneously.
// If ctx is nil, context.Background() is used.
func NewRunner[R any](ctx context.Context, concurrency int) *Runner[R] {
	if ctx == nil {
		ctx = context.Background()
	}
	childCtx, cancel := context.WithCancel(ctx)
	pool := pond.NewPool(concurrency, pond.WithContext(childCtx))
	return &Runner[R]{
		pool:   pool,
		ctx:    childCtx,
		cancel: cancel,
	}
}

// Stop cancels the runner's context and waits for all in-flight tasks to complete.
// It should be called when the runner is no longer needed to free resources.
// After calling Stop, the runner should not be reused.
func (r *Runner[R]) Stop() {
	r.cancel()
	r.pool.StopAndWait()
}

// run submits tasks and returns a channel of results (in completion order).
func (r *Runner[R]) run(tasks []func(context.Context) (R, error)) <-chan TaskResult[R] {
	if len(tasks) == 0 {
		ch := make(chan TaskResult[R])
		close(ch)
		return ch
	}

	ch := make(chan TaskResult[R], len(tasks))
	for i, task := range tasks {
		taskFn := task
		idx := i
		r.pool.Submit(func() {
			res, err := taskFn(r.ctx)
			select {
			case ch <- TaskResult[R]{Index: idx, Result: res, Err: err}:
			case <-r.ctx.Done():
			}
		})
	}

	go func() {
		r.pool.StopAndWait()
		close(ch)
	}()

	return ch
}

// ResultsChannel returns a channel that emits task results as they complete.
// Results are delivered in completion order, not in the original task order.
// The channel is closed when all tasks complete.
//
// Advanced API: The runner is NOT automatically stopped. Caller is responsible
// for lifecycle management (call Stop when appropriate).
func (r *Runner[R]) ResultsChannel(tasks []func(context.Context) (R, error)) <-chan TaskResult[R] {
	return r.run(tasks)
}

// FirstCompleted returns the result of the first task that completes,
// regardless of whether it succeeded or failed. Once a result is received,
// remaining tasks are cancelled and the runner is stopped.
//
// Returns ErrEmptyTasks if the task slice is empty.
func (r *Runner[R]) FirstCompleted(tasks []func(context.Context) (R, error)) TaskResult[R] {
	if len(tasks) == 0 {
		return TaskResult[R]{Err: ErrEmptyTasks}
	}
	ch := r.run(tasks)
	first := <-ch
	r.Stop()
	return first
}

// FirstSuccess returns the first task result with Err == nil.
// If all tasks fail, returns ErrNoSuccess.
// Once a successful result is received, remaining tasks are cancelled.
//
// Useful for redundant operations (e.g., trying multiple backends, fastest mirror).
// Returns ErrEmptyTasks if the task slice is empty.
func (r *Runner[R]) FirstSuccess(tasks []func(context.Context) (R, error)) TaskResult[R] {
	if len(tasks) == 0 {
		return TaskResult[R]{Err: ErrEmptyTasks}
	}
	ch := r.run(tasks)

	failed := 0
	total := len(tasks)

	for res := range ch {
		if res.Err == nil {
			r.Stop()
			return res
		}
		failed++
		if failed == total {
			r.Stop()
			return TaskResult[R]{Err: ErrNoSuccess}
		}
	}
	return TaskResult[R]{Err: ErrNoSuccess}
}

// FirstError returns the first task result with Err != nil.
// If all tasks succeed, returns ErrNoFailure.
// Once a failed result is received, remaining tasks are cancelled.
//
// Useful for fail-fast testing scenarios.
// Returns ErrEmptyTasks if the task slice is empty.
func (r *Runner[R]) FirstError(tasks []func(context.Context) (R, error)) TaskResult[R] {
	if len(tasks) == 0 {
		return TaskResult[R]{Err: ErrEmptyTasks}
	}
	ch := r.run(tasks)

	succeeded := 0
	total := len(tasks)

	for res := range ch {
		if res.Err != nil {
			r.Stop()
			return res
		}
		succeeded++
		if succeeded == total {
			r.Stop()
			return TaskResult[R]{Err: ErrNoFailure}
		}
	}
	return TaskResult[R]{Err: ErrNoFailure}
}

// All runs all tasks and returns their results in the original order,
// but only if every task succeeds. If any task fails, returns the first error encountered.
//
// This is similar to errgroup.Group behavior: fail-fast semantics with ordered results.
// For collecting both successes and failures, use AllSettled instead.
func (r *Runner[R]) All(tasks []func(context.Context) (R, error)) ([]R, error) {
	results := r.AllSettled(tasks)
	out := make([]R, len(results))
	for _, res := range results {
		if res.Err != nil {
			return nil, res.Err // Consider wrapping: fmt.Errorf("task %d: %w", res.Index, res.Err)
		}
		out[res.Index] = res.Result
	}
	return out, nil
}

// AllSettled runs all tasks to completion and returns all results in the original order,
// regardless of success or failure. This is useful when you need to process all outcomes.
//
// Similar to Promise.allSettled() in JavaScript.
// Returns nil if the task slice is empty.
func (r *Runner[R]) AllSettled(tasks []func(context.Context) (R, error)) []TaskResult[R] {
	if len(tasks) == 0 {
		return nil
	}
	ch := r.run(tasks)
	results := make([]TaskResult[R], len(tasks))
	received := 0
	for res := range ch {
		results[res.Index] = res
		received++
		if received == len(tasks) {
			break
		}
	}
	return results
}

// ======================== Functional API (recommended for most use cases) ========================

// FirstCompleted runs tasks concurrently with bounded concurrency and returns the first completion.
// The runner is automatically created and stopped. Use Runner.FirstCompleted for pool reuse.
func FirstCompleted[R any](ctx context.Context, concurrency int, tasks []func(context.Context) (R, error)) TaskResult[R] {
	r := NewRunner[R](ctx, concurrency)
	defer r.Stop()
	return r.FirstCompleted(tasks)
}

// FirstSuccess runs tasks concurrently with bounded concurrency and returns the first success.
// The runner is automatically created and stopped. Use Runner.FirstSuccess for pool reuse.
func FirstSuccess[R any](ctx context.Context, concurrency int, tasks []func(context.Context) (R, error)) TaskResult[R] {
	r := NewRunner[R](ctx, concurrency)
	defer r.Stop()
	return r.FirstSuccess(tasks)
}

// FirstError runs tasks concurrently with bounded concurrency and returns the first error.
// The runner is automatically created and stopped. Use Runner.FirstError for pool reuse.
func FirstError[R any](ctx context.Context, concurrency int, tasks []func(context.Context) (R, error)) TaskResult[R] {
	r := NewRunner[R](ctx, concurrency)
	defer r.Stop()
	return r.FirstError(tasks)
}

// All runs tasks concurrently with bounded concurrency; succeeds only if all tasks succeed.
// The runner is automatically created and stopped. Use Runner.All for pool reuse.
func All[R any](ctx context.Context, concurrency int, tasks []func(context.Context) (R, error)) ([]R, error) {
	r := NewRunner[R](ctx, concurrency)
	defer r.Stop()
	return r.All(tasks)
}

// AllSettled runs all tasks concurrently with bounded concurrency and returns ordered results regardless of errors.
// The runner is automatically created and stopped. Use Runner.AllSettled for pool reuse.
func AllSettled[R any](ctx context.Context, concurrency int, tasks []func(context.Context) (R, error)) []TaskResult[R] {
	r := NewRunner[R](ctx, concurrency)
	defer r.Stop()
	return r.AllSettled(tasks)
}

// ResultsChannel returns a raw result channel with bounded concurrency (advanced; manual lifecycle management).
// Caller is responsible for managing the runner lifecycle.
func ResultsChannel[R any](ctx context.Context, concurrency int, tasks []func(context.Context) (R, error)) <-chan TaskResult[R] {
	r := NewRunner[R](ctx, concurrency)
	// Caller must manage runner lifecycle if needed
	return r.ResultsChannel(tasks)
}

// ======================== Batch Processor ========================

// BatchResults holds the outcomes of batched task processing.
// Items contains all task results with their original indices.
type BatchResults[R any] struct {
	Items []TaskResult[R]
}

// BatchProcessor executes tasks in batches with configurable concurrency and delays.
// It's designed for rate-limited operations (e.g., API calls with rate limits).
//
// Example:
//
//	bp := concur.NewBatchProcessor[User, Result](10, 5, 1*time.Second)
//	results := bp.Process(ctx, users, func(ctx context.Context, u User) (Result, error) {
//	    return callAPI(ctx, u)
//	})
type BatchProcessor[T any, R any] struct {
	batchSize      int           // Number of items to process per batch
	sleepDuration  time.Duration // Delay between batches
	maxConcurrency int           // Max concurrent tasks within a batch
}

// NewBatchProcessor creates a new batch processor with the specified configuration.
//   - batchSize: number of items to process in each batch
//   - maxConcurrency: maximum concurrent tasks within a batch
//   - sleep: duration to wait between batches (useful for rate limiting)
func NewBatchProcessor[T any, R any](batchSize, maxConcurrency int, sleep time.Duration) *BatchProcessor[T, R] {
	return &BatchProcessor[T, R]{
		batchSize:      batchSize,
		sleepDuration:  sleep,
		maxConcurrency: maxConcurrency,
	}
}

// Process executes items in batches with the configured concurrency and delays.
// Each batch is processed concurrently up to maxConcurrency, with sleepDuration
// pause between batches. The function respects context cancellation and stops
// processing remaining batches if the context is cancelled.
//
// The fn parameter is called for each item with the parent context.
// Results maintain their original indices in the returned BatchResults.
func (bp *BatchProcessor[T, R]) Process(
	ctx context.Context,
	items []T,
	fn func(context.Context, T) (R, error),
) BatchResults[R] {
	if len(items) == 0 {
		return BatchResults[R]{}
	}

	pool := pond.NewResultPool[TaskResult[R]](bp.maxConcurrency, pond.WithContext(ctx))
	defer pool.StopAndWait()

	var allResults []TaskResult[R]
	total := len(items)
	batches := (total + bp.batchSize - 1) / bp.batchSize

	for batchIdx := 0; batchIdx < batches; batchIdx++ {
		if ctx.Err() != nil {
			break
		}

		start := batchIdx * bp.batchSize
		end := start + bp.batchSize
		if end > total {
			end = total
		}
		batch := items[start:end]

		group := pool.NewGroup()
		for i, item := range batch {
			idx := start + i
			it := item
			group.Submit(func() TaskResult[R] {
				res, err := fn(ctx, it)
				return TaskResult[R]{Index: idx, Result: res, Err: err}
			})
		}

		batchRes, _ := group.Wait() // pond rarely returns errors
		allResults = append(allResults, batchRes...)

		if bp.sleepDuration > 0 && end < total && ctx.Err() == nil {
			select {
			case <-ctx.Done():
				return BatchResults[R]{Items: allResults}
			case <-time.After(bp.sleepDuration):
			}
		}
	}

	return BatchResults[R]{Items: allResults}
}
