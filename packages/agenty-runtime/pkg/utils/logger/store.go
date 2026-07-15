/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
