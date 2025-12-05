# concur

[![Go Reference](https://pkg.go.dev/badge/github.com/shaokun11/concur.svg)](https://pkg.go.dev/github.com/shaokun11/concur)
[![Go Report Card](https://goreportcard.com/badge/github.com/shaokun11/concur)](https://goreportcard.com/report/github.com/shaokun11/concur)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

`concur` is a lightweight, idiomatic Go library for running concurrent tasks with bounded concurrency. It provides simple, functional APIs inspired by common concurrency patterns while leveraging the excellent [pond](https://github.com/alitto/pond) worker pool under the hood.

## Why concur?

- **Simple API**: Clean, functional interfaces that feel natural in Go
- **Bounded Concurrency**: Control resource usage with configurable limits
- **Context-Aware**: First-class support for context cancellation
- **Type-Safe**: Fully generic implementation using Go 1.18+ generics
- **Battle-Tested**: Built on top of the robust pond worker pool
- **Zero Dependencies**: Only depends on pond

## Perfect for

- 🔄 Redundant operations (try multiple backends, fastest mirror wins)
- ⚡ Parallel data processing with backpressure control
- 🚦 Batched operations with rate limiting
- 🎯 Fail-fast or collect-all-results scenarios

## Installation

```bash
go get github.com/shaokun11/concur
```

## Quick Start

```go
package main

import (
 "context"
 "fmt"
 "time"

 "github.com/shaokun11/concur"
)

func main() {
 ctx := context.Background()

 // Try multiple backends, return first success
 tasks := []func(context.Context) (string, error){
  func(ctx context.Context) (string, error) {
   return fetchFromPrimary(ctx)
  },
  func(ctx context.Context) (string, error) {
   return fetchFromBackup(ctx)
  },
  func(ctx context.Context) (string, error) {
   return fetchFromCache(ctx)
  },
 }

 result := concur.FirstSuccess(ctx, 3, tasks)
 if result.Err != nil {
  fmt.Println("All backends failed:", result.Err)
  return
 }
 fmt.Println("Got data from backend", result.Index, ":", result.Result)
}
```

## Features

### Concurrency Patterns

- **FirstCompleted**: Returns the first task that completes (success or failure)
- **FirstSuccess**: Returns the first task that succeeds
- **FirstError**: Returns the first task that fails
- **All**: Returns all results only if every task succeeds (fail-fast)
- **AllSettled**: Returns all results regardless of success/failure
- **ResultsChannel**: Low-level channel-based API for custom patterns

### Batch Processing

Process items in batches with configurable concurrency and delays between batches:

```go
bp := concur.NewBatchProcessor[User, Result](
 10,              // batch size
 5,               // max concurrency per batch
 1*time.Second,   // delay between batches
)

results := bp.Process(ctx, users, func(ctx context.Context, u User) (Result, error) {
 return callRateLimitedAPI(ctx, u)
})
```

## Usage Examples

### FirstSuccess - Redundant Operations

Try multiple data sources and return the first successful result:

```go
ctx := context.Background()

tasks := []func(context.Context) (string, error){
 func(ctx context.Context) (string, error) {
  time.Sleep(100 * time.Millisecond)
  return "", errors.New("primary failed")
 },
 func(ctx context.Context) (string, error) {
  time.Sleep(50 * time.Millisecond)
  return "backup-data", nil
 },
 func(ctx context.Context) (string, error) {
  time.Sleep(200 * time.Millisecond)
  return "cache-data", nil
 },
}

result := concur.FirstSuccess(ctx, 10, tasks)
// result.Result == "backup-data", result.Index == 1
```

### All - Fail-Fast Processing

Run all tasks and get ordered results, but fail if any task fails:

```go
tasks := []func(context.Context) (int, error){
 func(ctx context.Context) (int, error) { return processItem(1) },
 func(ctx context.Context) (int, error) { return processItem(2) },
 func(ctx context.Context) (int, error) { return processItem(3) },
}

results, err := concur.All(ctx, 5, tasks)
if err != nil {
 // One or more tasks failed
 return err
}
// results contains [result1, result2, result3] in order
```

### AllSettled - Collect All Results

Run all tasks and collect both successes and failures:

```go
tasks := []func(context.Context) (string, error){
 func(ctx context.Context) (string, error) { return "ok", nil },
 func(ctx context.Context) (string, error) { return "", errors.New("failed") },
 func(ctx context.Context) (string, error) { return "ok2", nil },
}

results := concur.AllSettled(ctx, 10, tasks)
for _, r := range results {
 if r.Err != nil {
  fmt.Printf("Task %d failed: %v\n", r.Index, r.Err)
 } else {
  fmt.Printf("Task %d succeeded: %v\n", r.Index, r.Result)
 }
}
```

### BatchProcessor - Rate-Limited Operations

Process items in batches with delays between batches:

```go
ctx := context.Background()

users := []User{...} // 1000 users

bp := concur.NewBatchProcessor[User, Result](
 50,              // process 50 users per batch
 10,              // max 10 concurrent requests per batch
 2*time.Second,   // wait 2 seconds between batches
)

results := bp.Process(ctx, users, func(ctx context.Context, u User) (Result, error) {
 return callAPI(ctx, u) // API has rate limits
})

// Process results
for _, r := range results.Items {
 if r.Err != nil {
  fmt.Printf("User %d failed: %v\n", r.Index, r.Err)
 }
}
```

### Runner - Reusable Worker Pool

For scenarios requiring multiple operations with the same pool:

```go
ctx := context.Background()
runner := concur.NewRunner[string](ctx, 10)
defer runner.Stop()

// Operation 1
result1 := runner.FirstSuccess(tasks1)

// Operation 2
results2 := runner.AllSettled(tasks2)

// Operation 3
result3 := runner.FirstCompleted(tasks3)
```

### Context Cancellation

All functions respect context cancellation:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

result := concur.FirstSuccess(ctx, 10, tasks)
if errors.Is(result.Err, context.DeadlineExceeded) {
 fmt.Println("Operation timed out")
}
```

## API Reference

### Functional API (Recommended)

These functions create a worker pool, execute tasks, and clean up automatically:

```go
func FirstCompleted[R any](ctx context.Context, concurrency int, 
 tasks []func(context.Context) (R, error)) TaskResult[R]

func FirstSuccess[R any](ctx context.Context, concurrency int, 
 tasks []func(context.Context) (R, error)) TaskResult[R]

func FirstError[R any](ctx context.Context, concurrency int, 
 tasks []func(context.Context) (R, error)) TaskResult[R]

func All[R any](ctx context.Context, concurrency int, 
 tasks []func(context.Context) (R, error)) ([]R, error)

func AllSettled[R any](ctx context.Context, concurrency int, 
 tasks []func(context.Context) (R, error)) []TaskResult[R]
```

### Runner API (Advanced)

For reusing the same worker pool across multiple operations:

```go
type Runner[R any] struct { ... }

func NewRunner[R any](ctx context.Context, concurrency int) *Runner[R]
func (r *Runner[R]) Stop()
func (r *Runner[R]) FirstCompleted(tasks) TaskResult[R]
func (r *Runner[R]) FirstSuccess(tasks) TaskResult[R]
func (r *Runner[R]) FirstError(tasks) TaskResult[R]
func (r *Runner[R]) All(tasks) ([]R, error)
func (r *Runner[R]) AllSettled(tasks) []TaskResult[R]
```

### Batch Processor

```go
type BatchProcessor[T any, R any] struct { ... }

func NewBatchProcessor[T, R any](batchSize, maxConcurrency int, 
 sleep time.Duration) *BatchProcessor[T, R]

func (bp *BatchProcessor[T, R]) Process(ctx context.Context, items []T, 
 fn func(context.Context, T) (R, error)) BatchResults[R]
```

### Types

```go
type TaskResult[R any] struct {
 Index  int   // Index of the task in the original task slice
 Result R     // Result value (only valid when Err == nil)
 Err    error // Error returned by the task (nil on success)
}

type BatchResults[R any] struct {
 Items []TaskResult[R]
}
```

### Errors

```go
var (
 ErrNoSuccess  = errors.New("no task succeeded")
 ErrNoFailure  = errors.New("no task failed")
 ErrEmptyTasks = errors.New("tasks list is empty")
)
```

## Comparison with Alternatives

| Feature | concur | errgroup | golang.org/x/sync/errgroup |
|---------|--------|----------|----------------------------|
| Bounded concurrency | ✅ | ❌ | ✅ (with semaphore) |
| First success pattern | ✅ | ❌ | ❌ |
| All settled pattern | ✅ | ❌ | ❌ |
| Batch processing | ✅ | ❌ | ❌ |
| Type-safe generics | ✅ | ❌ | ❌ |
| Context cancellation | ✅ | ✅ | ✅ |
| Result ordering | ✅ | ❌ | ❌ |

## Performance

- Built on [pond](https://github.com/alitto/pond), a high-performance worker pool
- Minimal overhead over raw goroutines
- Efficient result collection with buffered channels
- See `concur_test.go` for benchmarks

## Contributing

Contributions are welcome! Please feel free to submit issues or pull requests.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Acknowledgments

- Built on top of [pond](https://github.com/alitto/pond) by @alitto
- Inspired by JavaScript's Promise.race(), Promise.all(), and Promise.allSettled()
- Influenced by Go's errgroup pattern

## Related Projects

- [pond](https://github.com/alitto/pond) - Fast and flexible worker pool
- [errgroup](https://pkg.go.dev/golang.org/x/sync/errgroup) - Goroutine error groups
- [conc](https://github.com/sourcegraph/conc) - Better structured concurrency for Go
