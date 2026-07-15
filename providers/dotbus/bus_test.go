package dotbus

import (
	"sync"
	"testing"
	"time"
)

func TestPublishSubscribe(t *testing.T) {
	b := New(4)
	defer b.Shutdown()

	ch, err := b.Subscribe("chat")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := b.Publish("chat", "hello"); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	select {
	case got := <-ch:
		if got != "hello" {
			t.Fatalf("got %q, want hello", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestMPMCFanout(t *testing.T) {
	b := New(64)
	defer b.Shutdown()

	const nSub = 5
	const nPub = 3
	const nMsg = 10
	want := nPub * nMsg

	var readerWG sync.WaitGroup
	counts := make([]int, nSub)
	for i := 0; i < nSub; i++ {
		ch, err := b.Subscribe("t")
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
		readerWG.Add(1)
		go func(idx int, ch <-chan string) {
			defer readerWG.Done()
			for range ch {
				counts[idx]++
				if counts[idx] >= want {
					return
				}
			}
		}(i, ch)
	}

	var pubWG sync.WaitGroup
	for i := 0; i < nPub; i++ {
		pubWG.Add(1)
		go func() {
			defer pubWG.Done()
			for j := 0; j < nMsg; j++ {
				if err := b.Publish("t", "x"); err != nil {
					t.Errorf("Publish: %v", err)
					return
				}
			}
		}()
	}
	pubWG.Wait()

	done := make(chan struct{})
	go func() {
		readerWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout; counts=%v want %d each", counts, want)
	}
	for i, c := range counts {
		if c != want {
			t.Errorf("sub %d got %d, want %d", i, c, want)
		}
	}
}

func TestDropWhenFull(t *testing.T) {
	b := New(1)
	defer b.Shutdown()

	ch, err := b.Subscribe("t")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	// Fill buffer without reading.
	if err := b.Publish("t", "a"); err != nil {
		t.Fatalf("Publish a: %v", err)
	}
	// Must not block; second message dropped for this sub.
	done := make(chan struct{})
	go func() {
		_ = b.Publish("t", "b")
		_ = b.Publish("t", "c")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on full subscriber")
	}
	got := <-ch
	if got != "a" {
		t.Fatalf("first msg = %q, want a", got)
	}
}

func TestTopicIsolation(t *testing.T) {
	b := New(2)
	defer b.Shutdown()

	a, err := b.Subscribe("a")
	if err != nil {
		t.Fatal(err)
	}
	other, err := b.Subscribe("b")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Publish("a", "only-a"); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-a:
		if msg != "only-a" {
			t.Fatalf("got %q", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout on a")
	}
	select {
	case msg := <-other:
		t.Fatalf("topic b unexpectedly got %q", msg)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestUnsubscribeClosesChannel(t *testing.T) {
	b := New(2)
	defer b.Shutdown()

	ch, err := b.Subscribe("t")
	if err != nil {
		t.Fatal(err)
	}
	b.Unsubscribe(ch)
	if _, ok := <-ch; ok {
		t.Fatal("expected closed channel")
	}
	// Idempotent.
	b.Unsubscribe(ch)
}

func TestShutdown(t *testing.T) {
	b := New(2)
	ch, err := b.Subscribe("t")
	if err != nil {
		t.Fatal(err)
	}
	b.Shutdown()
	if _, ok := <-ch; ok {
		t.Fatal("expected closed channel after Shutdown")
	}
	if err := b.Publish("t", "x"); err == nil {
		t.Fatal("Publish after Shutdown should error")
	}
	if _, err := b.Subscribe("t"); err == nil {
		t.Fatal("Subscribe after Shutdown should error")
	}
	b.Shutdown() // idempotent
}

func TestEmptyTopic(t *testing.T) {
	b := New(1)
	defer b.Shutdown()
	if err := b.Publish("", "x"); err == nil {
		t.Fatal("Publish empty topic should error")
	}
	if _, err := b.Subscribe(""); err == nil {
		t.Fatal("Subscribe empty topic should error")
	}
}

func TestDefaultBuffer(t *testing.T) {
	b := New(0)
	defer b.Shutdown()
	if b.buffer != DefaultBuffer {
		t.Fatalf("buffer = %d, want %d", b.buffer, DefaultBuffer)
	}
}
