package openclawstate

import (
	"sync"
	"time"
)

var (
	mu                  sync.RWMutex
	lastMessageActivity time.Time
)

func MarkMessageActivity() {
	mu.Lock()
	lastMessageActivity = time.Now()
	mu.Unlock()
}

func RecentMessageActivityWithin(window time.Duration) bool {
	mu.RLock()
	last := lastMessageActivity
	mu.RUnlock()
	if last.IsZero() {
		return false
	}
	return time.Since(last) <= window
}
