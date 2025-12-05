package concur_test

import (
	"context"
	"fmt"
	"time"

	"github.com/shaokun11/concur"
)

func ExampleFirstSuccess() {
	ctx := context.Background()

	// Example 1: First success (e.g., multi-backend failover)
	tasks := []func(context.Context) (string, error){
		func(ctx context.Context) (string, error) { time.Sleep(300 * time.Millisecond); return "slow", nil },
		func(ctx context.Context) (string, error) { time.Sleep(100 * time.Millisecond); return "fast", nil },
		func(ctx context.Context) (string, error) { return "", fmt.Errorf("failed") },
	}

	result := concur.FirstSuccess(ctx, 10, tasks)
	if result.Err != nil {
		fmt.Println("All failed:", result.Err)
	} else {
		fmt.Println("Winner:", result.Result)
	}
	// Output: Winner: fast
}

func ExampleAll() {
	ctx := context.Background()

	tasks := []func(context.Context) (string, error){
		func(ctx context.Context) (string, error) { time.Sleep(300 * time.Millisecond); return "slow", nil },
		func(ctx context.Context) (string, error) { time.Sleep(100 * time.Millisecond); return "fast", nil },
		func(ctx context.Context) (string, error) { return "", fmt.Errorf("failed") },
	}

	results, err := concur.All(ctx, 5, tasks)
	if err != nil {
		fmt.Println("At least one task failed:", err)
	} else {
		fmt.Println("All results:", results)
	}
	// Output: At least one task failed: failed
}

func ExampleAllSettled() {
	ctx := context.Background()

	tasks := []func(context.Context) (string, error){
		func(ctx context.Context) (string, error) { return "success1", nil },
		func(ctx context.Context) (string, error) { return "", fmt.Errorf("error1") },
		func(ctx context.Context) (string, error) { return "success2", nil },
	}

	settled := concur.AllSettled(ctx, 5, tasks)
	for _, r := range settled {
		if r.Err != nil {
			fmt.Printf("Task %d: error\n", r.Index)
		} else {
			fmt.Printf("Task %d: %s\n", r.Index, r.Result)
		}
	}
	// Output:
	// Task 0: success1
	// Task 1: error
	// Task 2: success2
}

func ExampleBatchProcessor() {
	ctx := context.Background()

	users := []string{"user1", "user2", "user3", "user4", "user5"}
	bp := concur.NewBatchProcessor[string, string](2, 3, 100*time.Millisecond)

	results := bp.Process(ctx, users, func(ctx context.Context, user string) (string, error) {
		return "processed-" + user, nil
	})

	fmt.Printf("Processed %d users\n", len(results.Items))
	// Output: Processed 5 users
}
