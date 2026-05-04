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
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func withFindContainerByIP(t *testing.T, fn func(string) (*ProxyLookupResponse, error)) {
	original := findContainerByIPFn
	findContainerByIPFn = fn
	t.Cleanup(func() { findContainerByIPFn = original })
}

func TestHandleProxyGetSettingsMethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/settings", nil)
	rec := httptest.NewRecorder()

	handleProxyGetSettings(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want status %d (%s), got %d (%s)", http.StatusMethodNotAllowed, http.StatusText(http.StatusMethodNotAllowed), rec.Code, http.StatusText(rec.Code))
	}
}

func TestHandleContainerLookupByIPBadMethod(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/container-by-ip", nil)
	rec := httptest.NewRecorder()

	handleContainerLookupByIP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want status %d (%s), got %d (%s)", http.StatusMethodNotAllowed, http.StatusText(http.StatusMethodNotAllowed), rec.Code, http.StatusText(rec.Code))
	}
}

func TestHandleContainerLookupByIPBadPayload(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/container-by-ip", bytes.NewBufferString("not-json"))
	rec := httptest.NewRecorder()

	handleContainerLookupByIP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want status %d (%s), got %d (%s)", http.StatusBadRequest, http.StatusText(http.StatusBadRequest), rec.Code, http.StatusText(rec.Code))
	}
}

func TestHandleContainerLookupByIPEmptyIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/container-by-ip", bytes.NewBufferString(`{"ip":""}`))
	rec := httptest.NewRecorder()

	handleContainerLookupByIP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want status %d (%s), got %d (%s)", http.StatusBadRequest, http.StatusText(http.StatusBadRequest), rec.Code, http.StatusText(rec.Code))
	}
}

func TestHandleContainerLookupByIPNotFound(t *testing.T) {
	withFindContainerByIP(t, func(_ string) (*ProxyLookupResponse, error) {
		return nil, nil
	})

	req := httptest.NewRequest(http.MethodPost, "/container-by-ip", bytes.NewBufferString(`{"ip":"10.0.0.10"}`))
	rec := httptest.NewRecorder()

	handleContainerLookupByIP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want status %d (%s), got %d (%s)", http.StatusNotFound, http.StatusText(http.StatusNotFound), rec.Code, http.StatusText(rec.Code))
	}
}

func TestHandleContainerLookupByIPError(t *testing.T) {
	withFindContainerByIP(t, func(_ string) (*ProxyLookupResponse, error) {
		return nil, errors.New("boom")
	})

	req := httptest.NewRequest(http.MethodPost, "/container-by-ip", bytes.NewBufferString(`{"ip":"10.0.0.11"}`))
	rec := httptest.NewRecorder()

	handleContainerLookupByIP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want status %d (%s), got %d (%s)", http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError), rec.Code, http.StatusText(rec.Code))
	}
}

func TestHandleContainerLookupByIPSuccess(t *testing.T) {
	withFindContainerByIP(t, func(ip string) (*ProxyLookupResponse, error) {
		return &ProxyLookupResponse{
			ContainerID: "abc123",
			Name:        "/my-container",
			Labels:      map[string]string{"env": "test"},
		}, nil
	})

	req := httptest.NewRequest(http.MethodPost, "/container-by-ip", bytes.NewBufferString(`{"ip":"10.0.0.12"}`))
	rec := httptest.NewRecorder()

	handleContainerLookupByIP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want status %d (%s), got %d (%s)", http.StatusOK, http.StatusText(http.StatusOK), rec.Code, http.StatusText(rec.Code))
	}
}
