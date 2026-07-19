package logger

import "sync"

const maxStoredLogs = 2000

type logEntryStore struct {
	mu      sync.RWMutex
	entries []string
}

var entryStore = &logEntryStore{
	entries: make([]string, 0, 256),
}

func (s *logEntryStore) add(entry string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.entries) >= maxStoredLogs {
		copy(s.entries, s.entries[1:])
		s.entries = s.entries[:len(s.entries)-1]
	}
	s.entries = append(s.entries, entry)
}

func (s *logEntryStore) getAll() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]string, len(s.entries))
	copy(result, s.entries)
	return result
}

func GetStoredLogs() []string {
	return entryStore.getAll()
}
