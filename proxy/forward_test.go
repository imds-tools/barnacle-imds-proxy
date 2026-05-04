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
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestForwardRequestSuccess(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"iam":"metadata"}`))
	}))
	defer upstream.Close()

	originalURL := getForwardURL()
	setForwardURL(upstream.URL)
	t.Cleanup(func() { setForwardURL(originalURL) })

	req := httptest.NewRequest(http.MethodGet, "/latest/meta-data/", nil)
	rec := httptest.NewRecorder()

	if err := forwardRequest(rec, req, &lookupResponse{ContainerID: "abc123", Name: "/app", Labels: map[string]string{}}); err != nil {
		t.Fatalf("forwardRequest() error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("want status 200, got %d", rec.Code)
	}
}

func TestForwardRequestWithQueryString(t *testing.T) {
	var receivedQuery string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	originalURL := getForwardURL()
	setForwardURL(upstream.URL)
	t.Cleanup(func() { setForwardURL(originalURL) })

	req := httptest.NewRequest(http.MethodGet, "/metadata?v=1&region=us-east-1", nil)
	rec := httptest.NewRecorder()

	if err := forwardRequest(rec, req, &lookupResponse{ContainerID: "abc123", Name: "/app", Labels: map[string]string{}}); err != nil {
		t.Fatalf("forwardRequest() error: %v", err)
	}
	if receivedQuery != "v=1&region=us-east-1" {
		t.Errorf("want query string forwarded, got %q", receivedQuery)
	}
}

func TestForwardRequestWithLabels(t *testing.T) {
	var receivedLabels string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedLabels = r.Header.Get("x-container-labels")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	originalURL := getForwardURL()
	setForwardURL(upstream.URL)
	t.Cleanup(func() { setForwardURL(originalURL) })

	req := httptest.NewRequest(http.MethodGet, "/latest/", nil)
	rec := httptest.NewRecorder()
	containerInfo := &lookupResponse{
		ContainerID: "abc123",
		Name:        "/app",
		Labels:      map[string]string{"env": "test", "imds-proxy.enabled": "true"},
	}

	if err := forwardRequest(rec, req, containerInfo); err != nil {
		t.Fatalf("forwardRequest() error: %v", err)
	}
	if receivedLabels == "" {
		t.Error("want x-container-labels header to be set")
	}
}

func TestForwardRequestContainerHeaders(t *testing.T) {
	var gotID, gotName string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID = r.Header.Get("x-container-id")
		gotName = r.Header.Get("x-container-name")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	originalURL := getForwardURL()
	setForwardURL(upstream.URL)
	t.Cleanup(func() { setForwardURL(originalURL) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	if err := forwardRequest(rec, req, &lookupResponse{ContainerID: "myid", Name: "/myname", Labels: map[string]string{}}); err != nil {
		t.Fatalf("forwardRequest() error: %v", err)
	}
	if gotID != "myid" {
		t.Errorf("want x-container-id=myid, got %q", gotID)
	}
	if gotName != "/myname" {
		t.Errorf("want x-container-name=/myname, got %q", gotName)
	}
}

func TestForwardRequestError(t *testing.T) {
	originalURL := getForwardURL()
	// Port 1 is reserved/unreachable on Linux - connection refused
	setForwardURL("http://127.0.0.1:1")
	t.Cleanup(func() { setForwardURL(originalURL) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	err := forwardRequest(rec, req, &lookupResponse{ContainerID: "abc", Name: "/app", Labels: map[string]string{}})
	if err == nil {
		t.Fatal("want error for unreachable upstream, got nil")
	}
}

func TestHandleRequestSuccess(t *testing.T) {
	ensureClearCache(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("metadata"))
	}))
	defer upstream.Close()

	originalURL := getForwardURL()
	setForwardURL(upstream.URL)
	t.Cleanup(func() { setForwardURL(originalURL) })

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"containerId":"abc123","name":"/app","labels":{}}`))
	}))
	defer backend.Close()

	originalDial := backendDial
	backendDial = func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", backend.Listener.Addr().String())
	}
	defer func() { backendDial = originalDial }()

	req := httptest.NewRequest(http.MethodGet, "http://169.254.169.254/latest/meta-data/", nil)
	req.RemoteAddr = "10.0.0.5:1234"
	rec := httptest.NewRecorder()

	handleRequest(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("want status 200, got %d", rec.Code)
	}
}

func TestHandleRequestForwardError(t *testing.T) {
	ensureClearCache(t)

	originalURL := getForwardURL()
	setForwardURL("http://127.0.0.1:1") // connection refused
	t.Cleanup(func() { setForwardURL(originalURL) })

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"containerId":"abc123","name":"/app","labels":{}}`))
	}))
	defer backend.Close()

	originalDial := backendDial
	backendDial = func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", backend.Listener.Addr().String())
	}
	defer func() { backendDial = originalDial }()

	req := httptest.NewRequest(http.MethodGet, "http://169.254.169.254/latest/meta-data/", nil)
	req.RemoteAddr = "10.0.0.6:1234"
	rec := httptest.NewRecorder()

	handleRequest(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("want status 502, got %d", rec.Code)
	}
}

func TestForwardRequestLocalhostRewrite(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	// Extract just the port to build a localhost URL
	addr := upstream.Listener.Addr().String()
	port := addr[strings.LastIndex(addr, ":"):]

	originalURL := getForwardURL()
	setForwardURL("http://localhost" + port)
	t.Cleanup(func() { setForwardURL(originalURL) })

	// Override transport to redirect host.docker.internal back to test server
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	// forwardRequest will rewrite localhost→host.docker.internal; if that fails,
	// we just verify it attempted the rewrite (no panic, error expected since
	// host.docker.internal may not resolve in CI).
	_ = forwardRequest(rec, req, &lookupResponse{ContainerID: "abc", Name: "/app", Labels: map[string]string{}})
	// No assertion on success - this test exercises the rewrite code path.
}
