package cli

import (
	"testing"
	"time"
)

func TestAwaitLogShippingNilChannelReturnsImmediately(t *testing.T) {
	start := time.Now()
	awaitLogShipping(nil)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("awaitLogShipping(nil) took %s, want immediate return", elapsed)
	}
}

func TestAwaitLogShippingReturnsWhenShippingFinishes(t *testing.T) {
	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		close(done)
	}()

	start := time.Now()
	awaitLogShipping(done)
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("awaitLogShipping took %s, want return shortly after channel close", elapsed)
	}
}
