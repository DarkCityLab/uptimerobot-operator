/*
Copyright 2026 DarkCityLab.

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

// Package apikey reads the UptimeRobot API token from a mounted Secret file.
//
// Mounting (rather than injecting via an env var) lets a rotated projected
// Secret be picked up without restarting the pod: the token is re-read from
// disk whenever the file's modification time changes.
package apikey

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// FileSource reads and caches a token from a file, refreshing when the file's
// modification time changes. It is safe for concurrent use.
type FileSource struct {
	path string

	mu      sync.Mutex
	cached  string
	modTime time.Time
	loaded  bool
}

// NewFileSource returns a FileSource backed by the file at path.
func NewFileSource(path string) *FileSource {
	return &FileSource{path: path}
}

// Token returns the current token, re-reading the file if it changed. It
// satisfies uptimerobot.TokenFunc.
func (s *FileSource) Token(_ context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	info, err := os.Stat(s.path)
	if err != nil {
		if s.loaded {
			// Transient stat failure: fall back to the last good value.
			return s.cached, nil
		}
		return "", fmt.Errorf("stat api key file %q: %w", s.path, err)
	}

	if s.loaded && info.ModTime().Equal(s.modTime) {
		return s.cached, nil
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if s.loaded {
			return s.cached, nil
		}
		return "", fmt.Errorf("read api key file %q: %w", s.path, err)
	}

	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("api key file %q is empty", s.path)
	}

	s.cached = token
	s.modTime = info.ModTime()
	s.loaded = true
	return token, nil
}

// Validate eagerly reads the file once so startup fails fast on a missing or
// empty token.
func (s *FileSource) Validate(ctx context.Context) error {
	_, err := s.Token(ctx)
	return err
}
