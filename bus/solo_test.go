package stream

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestSoloBusLockExpiry(t *testing.T) {
	bus, err := NewSolo()
	if err != nil {
		t.Fatalf("NewSolo() failed: %v", err)
	}

	ctx := context.Background()
	_, err = bus.Lock(ctx, "key", 500*time.Millisecond)
	if err != nil {
		t.Fatalf("Lock() failed: %v", err)
	}

	time.Sleep(time.Second)

	// Lock should be expired, new lock should succeed
	lock2, err := bus.Lock(ctx, "key", time.Second)
	if err != nil {
		t.Errorf("Lock() after expiry failed: %v", err)
	}
	if lock2 != nil {
		lock2.Unlock()
	}
}

func TestSoloBusKeepAlive(t *testing.T) {
	bus, err := NewSolo()
	if err != nil {
		t.Fatalf("NewSolo() failed: %v", err)
	}

	ctx := context.Background()
	lock, err := bus.Lock(ctx, "key", 500*time.Millisecond)
	if err != nil {
		t.Fatalf("Lock() failed: %v", err)
	}

	// Keep lock alive
	time.Sleep(250 * time.Millisecond)
	if err := lock.KeepAlive(); err != nil {
		t.Errorf("KeepAlive() failed: %v", err)
	}

	// Verify lock is still valid
	_, err = bus.Lock(ctx, "key", time.Second)
	if err == nil {
		t.Error("Lock() succeeded when key should still be locked")
	}

	lock.Unlock()
}

func TestSoloBusPubSub(t *testing.T) {
	bus, err := NewSolo()
	if err != nil {
		t.Fatalf("NewSolo() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)

	// Subscriber 1
	go func() {
		defer wg.Done()
		msg, err := bus.Recv(ctx, "topic1")
		if err != nil {
			t.Errorf("Recv() failed: %v", err)
			return
		}
		if string(msg) != "test message" {
			t.Errorf("got message %q, want %q", string(msg), "test message")
		}
	}()

	// Subscriber 2
	go func() {
		defer wg.Done()
		msg, err := bus.Recv(ctx, "topic1")
		if err != nil {
			t.Errorf("Recv() failed: %v", err)
			return
		}
		if string(msg) != "test message" {
			t.Errorf("got message %q, want %q", string(msg), "test message")
		}
	}()

	// Publisher
	time.Sleep(100 * time.Millisecond) // Allow subscribers to set up
	if err := bus.Send("topic1", []byte("test message")); err != nil {
		t.Errorf("Send() failed: %v", err)
	}

	wg.Wait()
}

func TestSoloBusRecvTimeout(t *testing.T) {
	bus, err := NewSolo()
	if err != nil {
		t.Fatalf("NewSolo() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = bus.Recv(ctx, "topic")
	if err == nil {
		t.Error("Recv() should timeout")
	}
}

func TestSoloBusConcurrent(t *testing.T) {
	bus, err := NewSolo()
	if err != nil {
		t.Fatalf("NewSolo() failed: %v", err)
	}

	const numRoutines = 10
	var wg sync.WaitGroup
	wg.Add(numRoutines)

	for i := 0; i < numRoutines; i++ {
		go func() {
			defer wg.Done()
			ctx := context.Background()
			lock, err := bus.Lock(ctx, "shared", time.Second)
			if err == nil {
				time.Sleep(10 * time.Millisecond)
				lock.Unlock()
			}
		}()
	}

	wg.Wait()
}
