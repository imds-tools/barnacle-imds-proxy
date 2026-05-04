// Copyright 2026 Matt Miller
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSettingsMissingFile(t *testing.T) {
	tempDir := t.TempDir()
	originalPath := settingsPath
	settingsPath = filepath.Join(tempDir, "settings.json")
	defer func() { settingsPath = originalPath }()

	if err := loadSettings(); err != nil {
		t.Fatalf("want no error, got %v", err)
	}
}

func TestLoadSettingsInvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	originalPath := settingsPath
	settingsPath = filepath.Join(tempDir, "settings.json")
	defer func() { settingsPath = originalPath }()

	if err := os.WriteFile(settingsPath, []byte("{invalid"), 0644); err != nil {
		t.Fatalf("failed to write settings: %v", err)
	}

	if err := loadSettings(); err == nil {
		t.Fatalf("want error for invalid JSON")
	}
}

func TestLoadSettingsValidJSON(t *testing.T) {
	tempDir := t.TempDir()
	originalPath := settingsPath
	settingsPath = filepath.Join(tempDir, "settings.json")
	defer func() { settingsPath = originalPath }()

	payload := []byte(`{"url":"http://example.com"}`)
	if err := os.WriteFile(settingsPath, payload, 0644); err != nil {
		t.Fatalf("failed to write settings: %v", err)
	}

	if err := loadSettings(); err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	if settings.Get().URL != "http://example.com" {
		t.Fatalf("want URL to be loaded, got %q", settings.Get().URL)
	}
}

func TestPersistSettingsWritesFile(t *testing.T) {
	tempDir := t.TempDir()
	originalPath := settingsPath
	settingsPath = filepath.Join(tempDir, "settings.json")
	defer func() { settingsPath = originalPath }()

	settings.Set(Settings{URL: "http://example.com"})

	if err := persistSettings(); err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings: %v", err)
	}

	if len(data) == 0 {
		t.Fatalf("want settings file to be written")
	}
}

func TestPersistSettingsWriteError(t *testing.T) {
	tempDir := t.TempDir()
	originalPath := settingsPath
	// Point settingsPath at a directory (not a file) so WriteFile fails
	settingsPath = tempDir
	defer func() { settingsPath = originalPath }()

	settings.Set(Settings{URL: "http://write-error.example.com"})
	defer func() { settings.Set(Settings{}) }()

	if err := persistSettings(); err == nil {
		t.Fatal("want error when writing to a directory path, got nil")
	}
}

func TestPersistSettingsNestedDir(t *testing.T) {
	tempDir := t.TempDir()
	originalPath := settingsPath
	settingsPath = filepath.Join(tempDir, "nested", "deep", "settings.json")
	defer func() { settingsPath = originalPath }()

	settings.Set(Settings{URL: "http://nested.example.com"})
	defer func() { settings.Set(Settings{}) }()

	if err := persistSettings(); err != nil {
		t.Fatalf("persistSettings() with nested dir returned error: %v", err)
	}

	if _, err := os.Stat(settingsPath); err != nil {
		t.Errorf("settings file not created at %s: %v", settingsPath, err)
	}
}
