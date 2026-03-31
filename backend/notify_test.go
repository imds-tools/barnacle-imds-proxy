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
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func startTestUnixServer(t *testing.T, socketPath string, handler http.Handler) {
	t.Helper()
	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: handler}
	go srv.Serve(ln) //nolint:errcheck
	t.Cleanup(func() {
		srv.Close()
		os.Remove(socketPath)
	})
}

func withProxyNotificationSocket(t *testing.T, path string) {
	t.Helper()
	old := proxyNotificationSocketPath
	proxyNotificationSocketPath = path
	t.Cleanup(func() { proxyNotificationSocketPath = old })
}

func TestNotifyProxyConfigUpdateSuccess(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "notify.sock")

	var called atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/config-updated", func(w http.ResponseWriter, r *http.Request) {
		called.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	startTestUnixServer(t, socketPath, mux)
	withProxyNotificationSocket(t, socketPath)

	notifyProxyConfigUpdate()

	if called.Load() == 0 {
		t.Error("want /config-updated to be called at least once")
	}
}

func TestNotifyProxyConfigUpdateFailure(t *testing.T) {
	withProxyNotificationSocket(t, "/tmp/nonexistent-notify-socket-12345.sock")

	// Should exhaust retries and return without hanging.
	// notifyProxyConfigUpdate has 3 retries with backoff starting at 100ms -
	// total worst-case ~700ms, acceptable in a unit test.
	notifyProxyConfigUpdate()
}

func TestNotifyProxyContainerDestroyedSuccess(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "notify2.sock")

	var receivedID string
	var called atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/container-destroyed", func(w http.ResponseWriter, r *http.Request) {
		called.Add(1)
		body, _ := io.ReadAll(r.Body)
		var payload map[string]string
		_ = json.Unmarshal(body, &payload)
		receivedID = payload["containerId"]
		w.WriteHeader(http.StatusOK)
	})
	startTestUnixServer(t, socketPath, mux)
	withProxyNotificationSocket(t, socketPath)

	notifyProxyContainerDestroyed("test-container-abc")

	if called.Load() == 0 {
		t.Error("want /container-destroyed to be called at least once")
	}
	if receivedID != "test-container-abc" {
		t.Errorf("want containerID %q, got %q", "test-container-abc", receivedID)
	}
}

func TestNotifyProxyContainerDestroyedFailure(t *testing.T) {
	withProxyNotificationSocket(t, "/tmp/nonexistent-destroyed-socket-12345.sock")

	// Should exhaust retries and return without hanging.
	notifyProxyContainerDestroyed("some-container")
}

func TestNotifyProxyConfigUpdateNonOK(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "notify-nonok.sock")

	mux := http.NewServeMux()
	mux.HandleFunc("/config-updated", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	startTestUnixServer(t, socketPath, mux)
	withProxyNotificationSocket(t, socketPath)

	// Should exhaust retries against the non-200 server and return without hanging.
	notifyProxyConfigUpdate()
}

func TestNotifyProxyContainerDestroyedNonOK(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "notify-destroyed-nonok.sock")

	mux := http.NewServeMux()
	mux.HandleFunc("/container-destroyed", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	startTestUnixServer(t, socketPath, mux)
	withProxyNotificationSocket(t, socketPath)

	// Should exhaust retries against the non-200 server and return without hanging.
	notifyProxyContainerDestroyed("some-container")
}
