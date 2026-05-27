package main

import "sync"

// Store persists per-device playlist position.
// The zero-value position means "show the first screen".
type Store interface {
	// Position returns the current playlist index for deviceID.
	// Returns 0 and false for devices never seen before.
	Position(deviceID string) (int, bool)
	// Advance increments the playlist position for deviceID, wrapping at length,
	// stores the new position, and returns it.
	Advance(deviceID string, length int) int
}

// memStore is an in-memory Store. Safe for concurrent use.
type memStore struct {
	mu        sync.Mutex
	positions map[string]int
}

func newMemStore() *memStore {
	return &memStore{positions: make(map[string]int)}
}

func (s *memStore) Position(deviceID string) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pos, ok := s.positions[deviceID]
	return pos, ok
}

func (s *memStore) Advance(deviceID string, length int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	pos := (s.positions[deviceID] + 1) % length
	s.positions[deviceID] = pos
	return pos
}
