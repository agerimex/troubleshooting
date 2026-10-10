package batch

import (
	"context"
	"sync"
	"testing"
	"time"
)

type recorder struct {
	mu      sync.Mutex
	batches [][]int
}

func (r *recorder) flush(items []int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.batches = append(r.batches, items)
}

func (r *recorder) snapshot() [][]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([][]int(nil), r.batches...)
}

func TestFlushesWhenFull(t *testing.T) {
	rec := &recorder{}
	b := New(3, time.Hour, rec.flush)
	for i := 0; i < 7; i++ {
		if err := b.Add(context.Background(), i); err != nil {
			t.Fatal(err)
		}
	}
	b.Close()

	got := rec.snapshot()
	if len(got) != 3 || len(got[0]) != 3 || len(got[1]) != 3 || len(got[2]) != 1 {
		t.Fatalf("batches = %v, want sizes [3 3 1] (the last one flushed by Close)", got)
	}
}

func TestFlushesOnInterval(t *testing.T) {
	rec := &recorder{}
	b := New(100, 20*time.Millisecond, rec.flush)
	defer b.Close()

	b.Add(context.Background(), 1)
	deadline := time.Now().Add(2 * time.Second)
	for len(rec.snapshot()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("a partial batch was not flushed by the interval")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestAddRespectsContextWhenQueueIsFull(t *testing.T) {
	block := make(chan struct{})
	b := New(1, time.Hour, func([]int) { <-block })
	defer func() { close(block); b.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	var err error
	// The first item is taken by the blocked flush; the rest fill the queue.
	for i := 0; i < 20 && err == nil; i++ {
		err = b.Add(ctx, i)
	}
	if err == nil {
		t.Fatal("Add never reported the full queue")
	}
}
