package main

import (
	"sync"
)

type ReadCounter struct {
	mu       sync.Mutex
	counts   map[string]int
	maxReads map[string]int
}

func newReadCounter() *ReadCounter {
	return &ReadCounter{
		counts:   make(map[string]int),
		maxReads: make(map[string]int),
	}
}

func (rc *ReadCounter) setMaxReads(id string, max int) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.maxReads[id] = max
}

func (rc *ReadCounter) incrementAndCheck(id string) bool {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	rc.counts[id]++
	max, exists := rc.maxReads[id]

	// if there's a max read count and we've reached it, return true
	if exists && rc.counts[id] >= max {
		delete(rc.counts, id)
		delete(rc.maxReads, id)
		return true
	}
	return false
}
