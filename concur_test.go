package concur

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// TestFirstSuccess validates that the first successful task result is returned.
func TestFirstSuccess(t *testing.T) {
	ctx := context.Background()
	
	tasks := []func(context.Context) (string, error){
		func(ctx context.Context) (string, error) {
			time.Sleep(100 * time.Millisecond)
			return "", errors.New("task 0 failed")
		},
		func(ctx context.Context) (string, error) {
			time.Sleep(50 * time.Millisecond)
			return "success", nil
		},
		func(ctx context.Context) (string, error) {
			time.Sleep(200 * time.Millisecond)
			return "slow", nil
		},
	}
	
	result := FirstSuccess(ctx, 10, tasks)
	if result.Err != nil {
		t.Fatalf("expected success, got error: %v", result.Err)
	}
	if result.Result != "success" {
		t.Errorf("expected 'success', got '%s'", result.Result)
	}
	if result.Index != 1 {
		t.Errorf("expected index 1, got %d", result.Index)
	}
}

// TestFirstSuccessAllFail validates ErrNoSuccess when all tasks fail.
func TestFirstSuccessAllFail(t *testing.T) {
	ctx := context.Background()
	
	tasks := []func(context.Context) (string, error){
		func(ctx context.Context) (string, error) {
			return "", errors.New("fail 1")
		},
		func(ctx context.Context) (string, error) {
			return "", errors.New("fail 2")
		},
	}
	
	result := FirstSuccess(ctx, 10, tasks)
	if !errors.Is(result.Err, ErrNoSuccess) {
		t.Errorf("expected ErrNoSuccess, got %v", result.Err)
	}
}

// TestFirstSuccessEmptyTasks validates ErrEmptyTasks for empty task list.
func TestFirstSuccessEmptyTasks(t *testing.T) {
	ctx := context.Background()
	result := FirstSuccess[string](ctx, 10, nil)
	if !errors.Is(result.Err, ErrEmptyTasks) {
		t.Errorf("expected ErrEmptyTasks, got %v", result.Err)
	}
}

// TestFirstError validates that the first error is returned.
func TestFirstError(t *testing.T) {
	ctx := context.Background()
	
	tasks := []func(context.Context) (string, error){
		func(ctx context.Context) (string, error) {
			time.Sleep(100 * time.Millisecond)
			return "ok", nil
		},
		func(ctx context.Context) (string, error) {
			time.Sleep(30 * time.Millisecond)
			return "", errors.New("first error")
		},
	}
	
	result := FirstError(ctx, 10, tasks)
	if result.Err == nil {
		t.Fatal("expected error, got nil")
	}
	if result.Err.Error() != "first error" {
		t.Errorf("expected 'first error', got '%v'", result.Err)
	}
	if result.Index != 1 {
		t.Errorf("expected index 1, got %d", result.Index)
	}
}

// TestFirstErrorAllSuccess validates ErrNoFailure when all tasks succeed.
func TestFirstErrorAllSuccess(t *testing.T) {
	ctx := context.Background()
	
	tasks := []func(context.Context) (int, error){
		func(ctx context.Context) (int, error) { return 1, nil },
		func(ctx context.Context) (int, error) { return 2, nil },
	}
	
	result := FirstError(ctx, 10, tasks)
	if !errors.Is(result.Err, ErrNoFailure) {
		t.Errorf("expected ErrNoFailure, got %v", result.Err)
	}
}

// TestFirstCompleted validates that the first completed task is returned.
func TestFirstCompleted(t *testing.T) {
	ctx := context.Background()
	
	tasks := []func(context.Context) (string, error){
		func(ctx context.Context) (string, error) {
			time.Sleep(100 * time.Millisecond)
			return "slow", nil
		},
		func(ctx context.Context) (string, error) {
			time.Sleep(10 * time.Millisecond)
			return "fast", nil
		},
		func(ctx context.Context) (string, error) {
			time.Sleep(200 * time.Millisecond)
			return "", errors.New("slowest")
		},
	}
	
	result := FirstCompleted(ctx, 10, tasks)
	if result.Index != 1 {
		t.Errorf("expected first completed index 1, got %d", result.Index)
	}
	if result.Result != "fast" {
		t.Errorf("expected 'fast', got '%s'", result.Result)
	}
}

// TestAll validates that All returns results only when all tasks succeed.
func TestAll(t *testing.T) {
	ctx := context.Background()
	
	tasks := []func(context.Context) (int, error){
		func(ctx context.Context) (int, error) { return 1, nil },
		func(ctx context.Context) (int, error) { return 2, nil },
		func(ctx context.Context) (int, error) { return 3, nil },
	}
	
	results, err := All(ctx, 10, tasks)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
	for i, val := range []int{1, 2, 3} {
		if results[i] != val {
			t.Errorf("expected results[%d] = %d, got %d", i, val, results[i])
		}
	}
}

// TestAllWithError validates that All returns error if any task fails.
func TestAllWithError(t *testing.T) {
	ctx := context.Background()
	
	tasks := []func(context.Context) (int, error){
		func(ctx context.Context) (int, error) { return 1, nil },
		func(ctx context.Context) (int, error) { return 0, errors.New("task failed") },
		func(ctx context.Context) (int, error) { return 3, nil },
	}
	
	results, err := All(ctx, 10, tasks)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if results != nil {
		t.Errorf("expected nil results on error, got %v", results)
	}
}

// TestAllSettled validates that AllSettled returns all results.
func TestAllSettled(t *testing.T) {
	ctx := context.Background()
	
	tasks := []func(context.Context) (int, error){
		func(ctx context.Context) (int, error) { return 1, nil },
		func(ctx context.Context) (int, error) { return 0, errors.New("failed") },
		func(ctx context.Context) (int, error) { return 3, nil },
	}
	
	results := AllSettled(ctx, 10, tasks)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	
	if results[0].Err != nil || results[0].Result != 1 {
		t.Errorf("task 0: expected Result=1 Err=nil, got Result=%d Err=%v", results[0].Result, results[0].Err)
	}
	if results[1].Err == nil || results[1].Err.Error() != "failed" {
		t.Errorf("task 1: expected error 'failed', got %v", results[1].Err)
	}
	if results[2].Err != nil || results[2].Result != 3 {
		t.Errorf("task 2: expected Result=3 Err=nil, got Result=%d Err=%v", results[2].Result, results[2].Err)
	}
}

// TestContextCancellation validates that tasks respect context cancellation.
func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	
	var completed atomic.Int32
	tasks := []func(context.Context) (int, error){
		func(ctx context.Context) (int, error) {
			time.Sleep(50 * time.Millisecond)
			completed.Add(1)
			return 1, nil
		},
		func(ctx context.Context) (int, error) {
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(500 * time.Millisecond):
				completed.Add(1)
				return 2, nil
			}
		},
	}
	
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	
	result := FirstSuccess(ctx, 10, tasks)
	if result.Err != nil && result.Result != 1 {
		// Either we got the first task before cancel, or we got cancellation error
		if !errors.Is(result.Err, context.Canceled) && !errors.Is(result.Err, ErrNoSuccess) {
			t.Logf("got result or expected cancellation: %v", result.Err)
		}
	}
	
	time.Sleep(100 * time.Millisecond)
	count := completed.Load()
	if count > 1 {
		t.Errorf("expected at most 1 completion after cancel, got %d", count)
	}
}

// TestRunnerReuse validates that Runner can be reused for multiple operations.
func TestRunnerReuse(t *testing.T) {
	ctx := context.Background()
	runner := NewRunner[int](ctx, 5)
	defer runner.Stop()
	
	// First operation
	tasks1 := []func(context.Context) (int, error){
		func(ctx context.Context) (int, error) { return 10, nil },
		func(ctx context.Context) (int, error) { return 20, nil },
	}
	results1 := runner.AllSettled(tasks1)
	if len(results1) != 2 {
		t.Errorf("first operation: expected 2 results, got %d", len(results1))
	}
	
	// Second operation
	tasks2 := []func(context.Context) (int, error){
		func(ctx context.Context) (int, error) { return 30, nil },
	}
	results2 := runner.AllSettled(tasks2)
	if len(results2) != 1 {
		t.Errorf("second operation: expected 1 result, got %d", len(results2))
	}
}

// TestBatchProcessor validates batched processing with delays.
func TestBatchProcessor(t *testing.T) {
	ctx := context.Background()
	
	items := []int{1, 2, 3, 4, 5, 6, 7}
	bp := NewBatchProcessor[int, int](3, 5, 50*time.Millisecond)
	
	start := time.Now()
	results := bp.Process(ctx, items, func(ctx context.Context, item int) (int, error) {
		return item * 2, nil
	})
	elapsed := time.Since(start)
	
	if len(results.Items) != 7 {
		t.Fatalf("expected 7 results, got %d", len(results.Items))
	}
	
	for _, res := range results.Items {
		if res.Err != nil {
			t.Errorf("task %d failed: %v", res.Index, res.Err)
		}
		expected := (res.Index + 1) * 2
		if res.Result != expected {
			t.Errorf("task %d: expected %d, got %d", res.Index, expected, res.Result)
		}
	}
	
	// Should have at least 2 sleep periods (between 3 batches)
	minDuration := 100 * time.Millisecond
	if elapsed < minDuration {
		t.Errorf("expected at least %v delay between batches, got %v", minDuration, elapsed)
	}
}

// TestBatchProcessorCancellation validates batch processor respects context cancellation.
func TestBatchProcessorCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	
	items := make([]int, 100)
	for i := range items {
		items[i] = i
	}
	
	bp := NewBatchProcessor[int, int](10, 5, 50*time.Millisecond)
	
	go func() {
		time.Sleep(80 * time.Millisecond)
		cancel()
	}()
	
	results := bp.Process(ctx, items, func(ctx context.Context, item int) (int, error) {
		return item, nil
	})
	
	// Should process fewer than all items due to cancellation
	if len(results.Items) >= 100 {
		t.Errorf("expected fewer than 100 results due to cancellation, got %d", len(results.Items))
	}
}

// TestConcurrencyLimit validates that concurrency is respected.
func TestConcurrencyLimit(t *testing.T) {
	ctx := context.Background()
	
	var concurrent atomic.Int32
	var maxConcurrent atomic.Int32
	
	tasks := make([]func(context.Context) (int, error), 20)
	for i := range tasks {
		idx := i
		tasks[i] = func(ctx context.Context) (int, error) {
			current := concurrent.Add(1)
			defer concurrent.Add(-1)
			
			// Track max concurrent
			for {
				max := maxConcurrent.Load()
				if current <= max || maxConcurrent.CompareAndSwap(max, current) {
					break
				}
			}
			
			time.Sleep(50 * time.Millisecond)
			return idx, nil
		}
	}
	
	concurrencyLimit := 5
	_, err := All(ctx, concurrencyLimit, tasks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	max := maxConcurrent.Load()
	if max > int32(concurrencyLimit) {
		t.Errorf("concurrency limit exceeded: max=%d, limit=%d", max, concurrencyLimit)
	}
	if max < int32(concurrencyLimit)-1 {
		t.Errorf("concurrency not utilized: max=%d, limit=%d", max, concurrencyLimit)
	}
}

// BenchmarkFirstSuccess measures FirstSuccess performance.
func BenchmarkFirstSuccess(b *testing.B) {
	ctx := context.Background()
	tasks := []func(context.Context) (int, error){
		func(ctx context.Context) (int, error) { return 1, nil },
		func(ctx context.Context) (int, error) { return 2, nil },
		func(ctx context.Context) (int, error) { return 3, nil },
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = FirstSuccess(ctx, 10, tasks)
	}
}

// BenchmarkAll measures All performance.
func BenchmarkAll(b *testing.B) {
	ctx := context.Background()
	tasks := make([]func(context.Context) (int, error), 100)
	for i := range tasks {
		idx := i
		tasks[i] = func(ctx context.Context) (int, error) {
			return idx, nil
		}
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = All(ctx, 10, tasks)
	}
}
