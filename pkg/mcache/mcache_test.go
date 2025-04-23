package mcache

import (
	"testing"
	"time"
)

func TestCache_SetAndGet(t *testing.T) {
	cache := New[string, string](time.Minute)

	cache.Set("key1", "value1")
	value, ok := cache.Get("key1")
	if !ok || value != "value1" {
		t.Errorf("expected value1, got %v", value)
	}
}

func TestCache_GetExpired(t *testing.T) {
	cache := New[string, string](time.Millisecond)

	cache.Set("key1", "value1")
	time.Sleep(2 * time.Millisecond)
	_, ok := cache.Get("key1")
	if ok {
		t.Error("expected key1 to be expired")
	}
}

func TestCache_Delete(t *testing.T) {
	cache := New[string, string](time.Minute)

	cache.Set("key1", "value1")
	deleted := cache.Delete("key1")
	if !deleted {
		t.Error("expected key1 to be deleted")
	}

	_, ok := cache.Get("key1")
	if ok {
		t.Error("expected key1 to be absent")
	}
}

func TestCache_Count(t *testing.T) {
	cache := New[string, string](time.Minute)

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	if count := cache.Count(); count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}
}

func TestCache_Cleanup(t *testing.T) {
	cache := New[string, string](time.Millisecond)

	cache.Set("key1", "value1")
	cache.Set("key2", "value2", 2*time.Millisecond)
	time.Sleep(3 * time.Millisecond)

	cleaned := cache.Cleanup()
	if cleaned != 2 {
		t.Errorf("expected 2 items cleaned, got %d", cleaned)
	}
}

func TestCache_Flush(t *testing.T) {
	cache := New[string, string](time.Minute)

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Flush()

	if count := cache.Count(); count != 0 {
		t.Errorf("expected count 0 after flush, got %d", count)
	}
}

func TestStartCleanupLoop(t *testing.T) {
	cache := New[string, string](time.Millisecond)

	cache.Set("key1", "value1")
	stop := StartCleanupLoop(cache, time.Millisecond)
	time.Sleep(3 * time.Millisecond)
	stop()

	if count := cache.Count(); count != 0 {
		t.Errorf("expected count 0 after cleanup loop, got %d", count)
	}
}
