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
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/network"
)

// monitorDockerClient extends fakeDockerClient with controllable event channels.
type monitorDockerClient struct {
	fakeDockerClient
	eventsCh chan events.Message
	errCh    chan error
}

func newMonitorDockerClient() *monitorDockerClient {
	return &monitorDockerClient{
		eventsCh: make(chan events.Message, 10),
		errCh:    make(chan error, 1),
	}
}

func (m *monitorDockerClient) Events(_ context.Context, _ events.ListOptions) (<-chan events.Message, <-chan error) {
	return m.eventsCh, m.errCh
}

func withDockerClient(t *testing.T, cli DockerClient) {
	t.Helper()
	old := dockerClient
	dockerClient = cli
	t.Cleanup(func() { dockerClient = old })
}

func withShutdownChan(t *testing.T) chan struct{} {
	t.Helper()
	old := shutdownChan
	ch := make(chan struct{})
	shutdownChan = ch
	t.Cleanup(func() { shutdownChan = old })
	return ch
}

// startMonitor starts monitorDockerEvents in a goroutine. It returns:
//   - finished: closed when the goroutine exits (use for in-test assertions)
//   - shutdown: call to close done exactly once (safe to call multiple times)
//
// A t.Cleanup registered here (LIFO relative to withShutdownChan /
// withDockerClient) calls shutdown and waits for finished, ensuring the
// goroutine has exited before those helpers restore their package-level vars.
func startMonitor(t *testing.T, done chan struct{}) (finished chan struct{}, shutdown func()) {
	t.Helper()
	finished = make(chan struct{})
	var once sync.Once
	shutdown = func() { once.Do(func() { close(done) }) }

	go func() {
		monitorDockerEvents()
		close(finished)
	}()

	t.Cleanup(func() {
		shutdown()
		select {
		case <-finished:
		case <-time.After(500 * time.Millisecond):
		}
	})
	return finished, shutdown
}

func TestListen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sock")

	ln, err := listen(path)
	if err != nil {
		t.Fatalf("listen() returned error: %v", err)
	}
	defer ln.Close()
	if ln == nil {
		t.Fatal("want non-nil listener")
	}
}

func TestMonitorDockerEventsShutdown(t *testing.T) {
	resetTracking()
	t.Cleanup(resetTracking)

	cli := newMonitorDockerClient()
	withDockerClient(t, cli)
	done := withShutdownChan(t)
	finished, shutdown := startMonitor(t, done)

	time.Sleep(20 * time.Millisecond)
	shutdown()

	select {
	case <-finished:
	case <-time.After(500 * time.Millisecond):
		t.Error("monitorDockerEvents did not return after shutdown signal")
	}
}

func TestMonitorDockerEventsContainerCreate(t *testing.T) {
	resetTracking()
	t.Cleanup(resetTracking)

	containerID := "monitor-create-abc"
	inspectResp := container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			ID:    containerID,
			Name:  "/monitor-test",
			State: &container.State{Running: false},
		},
		Config: &container.Config{Labels: map[string]string{"imds-proxy.enabled": "true"}},
		NetworkSettings: &container.NetworkSettings{
			Networks: map[string]*network.EndpointSettings{},
		},
	}

	cli := newMonitorDockerClient()
	cli.fakeDockerClient.inspectSequence = []container.InspectResponse{inspectResp, inspectResp}

	withDockerClient(t, cli)
	done := withShutdownChan(t)
	startMonitor(t, done)
	time.Sleep(20 * time.Millisecond)

	cli.eventsCh <- events.Message{
		Type:   events.ContainerEventType,
		Action: "create",
		Actor: events.Actor{
			ID: containerID,
			Attributes: map[string]string{
				"imds-proxy.enabled": "true",
				"image":              "test:latest",
			},
		},
	}
	time.Sleep(100 * time.Millisecond)

	tracker.mu.RLock()
	_, tracked := tracker.byID[containerID]
	tracker.mu.RUnlock()
	if !tracked {
		t.Errorf("want container %s tracked after create event", containerID)
	}
}

func TestMonitorDockerEventsContainerCreateNoLabel(t *testing.T) {
	resetTracking()
	t.Cleanup(resetTracking)

	containerID := "monitor-nolabel-abc"
	cli := newMonitorDockerClient()
	withDockerClient(t, cli)
	done := withShutdownChan(t)
	startMonitor(t, done)
	time.Sleep(20 * time.Millisecond)

	cli.eventsCh <- events.Message{
		Type:   events.ContainerEventType,
		Action: "create",
		Actor: events.Actor{
			ID:         containerID,
			Attributes: map[string]string{"image": "test:latest"},
		},
	}
	time.Sleep(50 * time.Millisecond)

	tracker.mu.RLock()
	_, tracked := tracker.byID[containerID]
	tracker.mu.RUnlock()
	if tracked {
		t.Errorf("want unlabeled container %s NOT tracked", containerID)
	}
}

func TestMonitorDockerEventsContainerDestroy(t *testing.T) {
	resetTracking()
	t.Cleanup(resetTracking)

	containerID := "monitor-destroy-abc"
	tracker.mu.Lock()
	tracker.byID[containerID] = ContainerInfo{ContainerID: containerID, Name: "/to-destroy"}
	tracker.mu.Unlock()

	cli := newMonitorDockerClient()
	withDockerClient(t, cli)
	done := withShutdownChan(t)
	startMonitor(t, done)
	time.Sleep(20 * time.Millisecond)

	cli.eventsCh <- events.Message{
		Type:   events.ContainerEventType,
		Action: "destroy",
		Actor:  events.Actor{ID: containerID},
	}
	time.Sleep(100 * time.Millisecond)

	tracker.mu.RLock()
	_, tracked := tracker.byID[containerID]
	tracker.mu.RUnlock()
	if tracked {
		t.Errorf("want container %s removed after destroy event", containerID)
	}
}

func TestMonitorDockerEventsNetworkConnect(t *testing.T) {
	resetTracking()
	t.Cleanup(resetTracking)

	containerID := "monitor-net-abc"
	tracker.mu.Lock()
	tracker.byID[containerID] = ContainerInfo{ContainerID: containerID, Name: "/net-test", Networks: []NetworkInfo{}}
	tracker.mu.Unlock()

	cli := newMonitorDockerClient()
	cli.fakeDockerClient.inspectSequence = []container.InspectResponse{
		{
			ContainerJSONBase: &container.ContainerJSONBase{
				ID:    containerID,
				Name:  "/net-test",
				State: &container.State{Running: true},
			},
			Config: &container.Config{Labels: map[string]string{}},
			NetworkSettings: &container.NetworkSettings{
				Networks: map[string]*network.EndpointSettings{
					"test-net": {NetworkID: "net-99"},
				},
			},
		},
	}
	withDockerClient(t, cli)
	done := withShutdownChan(t)
	startMonitor(t, done)
	time.Sleep(20 * time.Millisecond)

	cli.eventsCh <- events.Message{
		Type:   events.NetworkEventType,
		Action: "connect",
		Actor:  events.Actor{Attributes: map[string]string{"container": containerID}},
	}
	time.Sleep(100 * time.Millisecond)
}

func TestMonitorDockerEventsNetworkConnectUntracked(t *testing.T) {
	resetTracking()
	t.Cleanup(resetTracking)

	cli := newMonitorDockerClient()
	withDockerClient(t, cli)
	done := withShutdownChan(t)
	startMonitor(t, done)
	time.Sleep(20 * time.Millisecond)

	cli.eventsCh <- events.Message{
		Type:   events.NetworkEventType,
		Action: "disconnect",
		Actor:  events.Actor{Attributes: map[string]string{"container": "untracked-container"}},
	}
	time.Sleep(50 * time.Millisecond)
}

func TestMonitorDockerEventsErrChanEOF(t *testing.T) {
	resetTracking()
	t.Cleanup(resetTracking)

	cli := newMonitorDockerClient()
	withDockerClient(t, cli)
	done := withShutdownChan(t)
	finished, _ := startMonitor(t, done)
	time.Sleep(20 * time.Millisecond)

	cli.errCh <- io.EOF

	select {
	case <-finished:
	case <-time.After(500 * time.Millisecond):
		t.Error("monitorDockerEvents did not return after io.EOF")
	}
}

func TestMonitorDockerEventsErrChanError(t *testing.T) {
	resetTracking()
	t.Cleanup(resetTracking)

	cli := newMonitorDockerClient()
	withDockerClient(t, cli)
	done := withShutdownChan(t)
	finished, _ := startMonitor(t, done)
	time.Sleep(20 * time.Millisecond)

	cli.errCh <- errors.New("docker daemon disconnected")

	select {
	case <-finished:
	case <-time.After(500 * time.Millisecond):
		t.Error("monitorDockerEvents did not return after error")
	}
}

func TestStartProxySocketServer(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "proxy-test.sock")

	withSettings(t, Settings{URL: "http://proxy-server-test.example.com"})
	withDockerClient(t, &fakeDockerClient{})
	withFindContainerByIP(t, func(_ string) (*ProxyLookupResponse, error) {
		return nil, nil
	})

	go startProxySocketServer(socketPath)
	time.Sleep(50 * time.Millisecond)

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	}

	resp, err := client.Get("http://unix/settings")
	if err != nil {
		t.Fatalf("GET /settings through proxy socket: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}
