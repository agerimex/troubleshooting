package batch

import (
	"context"
	"time"
)

// Batcher collects items and hands them to flush when size items are buffered
// or every interval, whichever comes first. flush runs on a single goroutine,
// so inserts are never concurrent and a slow database applies back-pressure.
type Batcher[T any] struct {
	items chan T
	done  chan struct{}
}

func New[T any](size int, interval time.Duration, flush func([]T)) *Batcher[T] {
	b := &Batcher[T]{
		items: make(chan T, size*10),
		done:  make(chan struct{}),
	}
	go b.run(size, interval, flush)
	return b
}

// Add queues an item, waiting while the queue is full unless ctx is done.
// It must not be called after Close.
func (b *Batcher[T]) Add(ctx context.Context, item T) error {
	select {
	case b.items <- item:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close flushes everything still buffered and waits for that flush to finish.
func (b *Batcher[T]) Close() {
	close(b.items)
	<-b.done
}

func (b *Batcher[T]) run(size int, interval time.Duration, flush func([]T)) {
	defer close(b.done)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	buffer := make([]T, 0, size)
	send := func() {
		if len(buffer) == 0 {
			return
		}
		flush(buffer)
		buffer = make([]T, 0, size)
	}

	for {
		select {
		case item, ok := <-b.items:
			if !ok {
				send()
				return
			}
			buffer = append(buffer, item)
			if len(buffer) >= size {
				send()
			}
		case <-ticker.C:
			send()
		}
	}
}
