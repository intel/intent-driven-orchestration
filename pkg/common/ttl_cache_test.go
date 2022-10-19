package common

import (
	"testing"
	"time"
)

// Tests for success.

// TestPutForSuccess tests for success.
func TestPutForSuccess(t *testing.T) {
	cache, done := NewCache(10, time.Duration(100))
	defer close(done)
	cache.Put("foo")
}

// TestIsInForSuccess tests for success.
func TestIsInForSuccess(t *testing.T) {
	cache, done := NewCache(10, time.Duration(100))
	defer close(done)
	cache.Put("foo")
	cache.IsIn("foo")
}

// Tests for failure.

// N/A.

// Tests for sanity.

// TestPutForSanity tests for sanity.
func TestPutForSanity(t *testing.T) {
	cache, done := NewCache(10, time.Duration(50))
	defer close(done)
	cache.Put("foo")
	res := cache.IsIn("foo")
	if res != true {
		t.Errorf("foo should still be in the cache.")
	}
	time.Sleep(time.Duration(75) * time.Millisecond)
	res = cache.IsIn("foo")
	if res != false {
		t.Errorf("foo should not be in the cache.")
	}
}
