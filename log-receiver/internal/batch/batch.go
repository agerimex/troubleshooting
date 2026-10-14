package batch

import (
	"context"
	"time"
)

// Batcher collects items and hands them to flush when size items are buffered
// or every interval, whichever comes first. flush runs on a single goroutine,
// so inserts are never concurrent and a slow database applies back-pressure.
//
// Add waits for the flush that contains its item and returns that flush's
// error, so a caller never confirms an item that was not written.
type Batcher[T any] struct {
	items chan pending[T]
	done  chan struct{}
}

type pending[T any] struct {
	item   T
	result chan error
}

func New[T any](size int, interval time.Duration, flush func([]T) error) *Batcher[T] {
	b := &Batcher[T]{
		items: make(chan pending[T], size*10),
		done:  make(chan struct{}),
	}
	go b.run(size, interval, flush)
	return b
}

// Add queues an item and waits until it has been flushed. It returns the
// flush error, or ctx's error if ctx ends first: while the queue is full the
// item is not queued at all; after that it may still be written later, so the
// result is "unconfirmed" and the sender should retry with the same identity.
// It must not be called after Close.
func (b *Batcher[T]) Add(ctx context.Context, item T) error {
	p := pending[T]{item: item, result: make(chan error, 1)}
	select {
	case b.items <- p:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-p.result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close flushes everything still buffered and waits for that flush to finish.
func (b *Batcher[T]) Close() {
	close(b.items)
	<-b.done
}

func (b *Batcher[T]) run(size int, interval time.Duration, flush func([]T) error) {
	defer close(b.done)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	buffer := make([]pending[T], 0, size)
	send := func() {
		if len(buffer) == 0 {
			return
		}
		items := make([]T, len(buffer))
		for i, p := range buffer {
			items[i] = p.item
		}
		err := flush(items)
		for _, p := range buffer {
			p.result <- err
		}
		buffer = make([]pending[T], 0, size)
	}

	for {
		select {
		case p, ok := <-b.items:
			if !ok {
				send()
				return
			}
			buffer = append(buffer, p)
			if len(buffer) >= size {
				send()
			}
		case <-ticker.C:
			send()
		}
	}
}
