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

package apikey

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileSourceReadsAndTrims(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "apiKey")
	if err := os.WriteFile(path, []byte("  secret-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := NewFileSource(path)
	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok != "secret-token" {
		t.Errorf("want trimmed token, got %q", tok)
	}
}

func TestFileSourceEmptyIsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "apiKey")
	if err := os.WriteFile(path, []byte("   \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := NewFileSource(path).Validate(context.Background()); err == nil {
		t.Error("empty token file should error")
	}
}

func TestFileSourceMissingIsError(t *testing.T) {
	if err := NewFileSource("/nonexistent/apiKey").Validate(context.Background()); err == nil {
		t.Error("missing file should error")
	}
}

func TestFileSourcePicksUpRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "apiKey")
	if err := os.WriteFile(path, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := NewFileSource(path)
	if tok, _ := src.Token(context.Background()); tok != "first" {
		t.Fatalf("want first, got %q", tok)
	}

	// Rewrite with a newer mod time.
	future := time.Now().Add(2 * time.Second)
	if err := os.WriteFile(path, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	if tok, _ := src.Token(context.Background()); tok != "second" {
		t.Errorf("want rotated token second, got %q", tok)
	}
}
