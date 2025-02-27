package main

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

// Job represents a processing unit with response channel
type Job struct {
	ID      string
	Payload struct{}
	Result  chan int
}

var (
	jobQueue    = make(chan Job, 100)       // Buffered job channel
	workerWg    sync.WaitGroup              // Worker synchronization
	results     = make(map[string]chan int) // Active results
	resultsLock sync.Mutex                  // Map protection
)

func heavyJob() int {
	sum := 0
	for i := 0; i < 50_000_000; i++ {
		sum += rand.Intn(10)
	}
	return sum
}

// Worker pool implementation
func worker(ctx context.Context) {
	defer workerWg.Done()
	for {
		select {
		case job := <-jobQueue:
			result := heavyJob()
			job.Result <- result
		case <-ctx.Done():
			return
		}
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	resultChan := make(chan int, 1)
	jobID := fmt.Sprintf("job-%d", time.Now().UnixNano())

	// Store result channel before queuing
	resultsLock.Lock()
	results[jobID] = resultChan
	resultsLock.Unlock()

	// Submit job to worker pool
	jobQueue <- Job{
		ID:     jobID,
		Result: resultChan,
	}

	// Wait for result with timeout
	select {
	case res := <-resultChan:
		fmt.Fprintf(w, "Result: %d", res)
	case <-time.After(5 * time.Second):
		http.Error(w, "Processing timeout", http.StatusGatewayTimeout)
	}

	// Cleanup
	resultsLock.Lock()
	delete(results, jobID)
	resultsLock.Unlock()
	close(resultChan)
}

func main() {
	rand.Seed(time.Now().UnixNano())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start worker pool
	for i := 0; i < 5; i++ {
		workerWg.Add(1)
		go worker(ctx)
	}

	http.HandleFunc("/", handler)
	fmt.Println("Server starting on :8080...")

	// Graceful shutdown handling
	server := &http.Server{Addr: ":8080"}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Server error: %v\n", err)
		}
	}()

	// Handle shutdown
	<-ctx.Done()
	server.Shutdown(context.Background())
	workerWg.Wait()
}
