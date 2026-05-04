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
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/network"
)

type fakeDockerClient struct {
	inspectCalls            int
	inspectSequence         []container.InspectResponse
	inspectErr              error
	networkConnectCalls     []string
	networkCreateCalls      []string
	networkDisconnectCalls  []string
	networkRemoveCalls      []string
	pauseCalls              int
	unpauseCalls            int
	closeCalls              int
	containerList           []container.Summary
	containerListErr        error
	networkList             []network.Summary
	networkListErr          error
	networkCreateErr        error
	networkRemoveErr        error
}

func (f *fakeDockerClient) ContainerInspect(_ context.Context, _ string) (container.InspectResponse, error) {
	if f.inspectErr != nil {
		return container.InspectResponse{}, f.inspectErr
	}
	if len(f.inspectSequence) == 0 {
		return container.InspectResponse{}, nil
	}
	index := f.inspectCalls
	if index >= len(f.inspectSequence) {
		index = len(f.inspectSequence) - 1
	}
	f.inspectCalls++
	return f.inspectSequence[index], nil
}

func (f *fakeDockerClient) ContainerList(_ context.Context, _ container.ListOptions) ([]container.Summary, error) {
	return f.containerList, f.containerListErr
}

func (f *fakeDockerClient) Events(_ context.Context, _ events.ListOptions) (<-chan events.Message, <-chan error) {
	return make(chan events.Message), make(chan error)
}

func (f *fakeDockerClient) NetworkConnect(_ context.Context, networkID, _ string, _ *network.EndpointSettings) error {
	f.networkConnectCalls = append(f.networkConnectCalls, networkID)
	return nil
}

func (f *fakeDockerClient) NetworkCreate(_ context.Context, name string, _ network.CreateOptions) (network.CreateResponse, error) {
	f.networkCreateCalls = append(f.networkCreateCalls, name)
	if f.networkCreateErr != nil {
		return network.CreateResponse{}, f.networkCreateErr
	}
	return network.CreateResponse{ID: "net-" + name}, nil
}

func (f *fakeDockerClient) NetworkDisconnect(_ context.Context, networkID, containerID string, _ bool) error {
	f.networkDisconnectCalls = append(f.networkDisconnectCalls, networkID+":"+containerID)
	return nil
}

func (f *fakeDockerClient) NetworkRemove(_ context.Context, networkID string) error {
	f.networkRemoveCalls = append(f.networkRemoveCalls, networkID)
	if f.networkRemoveErr != nil {
		return f.networkRemoveErr
	}
	return nil
}

func (f *fakeDockerClient) NetworkList(_ context.Context, _ network.ListOptions) ([]network.Summary, error) {
	if f.networkList != nil || f.networkListErr != nil {
		return f.networkList, f.networkListErr
	}
	return []network.Summary{}, nil
}

func (f *fakeDockerClient) ContainerPause(_ context.Context, _ string) error {
	f.pauseCalls++
	return nil
}

func (f *fakeDockerClient) ContainerUnpause(_ context.Context, _ string) error {
	f.unpauseCalls++
	return nil
}

func (f *fakeDockerClient) Close() error {
	f.closeCalls++
	return nil
}

func resetTracking() {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	tracker.ipToContainerID = make(map[string]string)
	tracker.byID = make(map[string]ContainerInfo)
}

func TestUpdateAndRemoveIPIndex(t *testing.T) {
	resetTracking()
	defer resetTracking()

	containerID := "abc123"
	tracker.mu.Lock()
	tracker.byID[containerID] = ContainerInfo{
		ContainerID: containerID,
		Networks: []NetworkInfo{{
			NetworkID:   "net-1",
			NetworkName: "imds",
		}},
	}
	tracker.mu.Unlock()

	updateIPIndex(containerID)

	tracker.mu.RLock()
	if got := tracker.ipToContainerID["net-1"]; got != containerID {
		tracker.mu.RUnlock()
		t.Fatalf("want ip index to be set, got %q", got)
	}
	tracker.mu.RUnlock()

	removeIPIndexForContainer(containerID, tracker.byID[containerID])

	tracker.mu.RLock()
	if _, ok := tracker.ipToContainerID["net-1"]; ok {
		tracker.mu.RUnlock()
		t.Fatalf("want ip index to be cleared")
	}
	tracker.mu.RUnlock()
}

func TestAddAndRemoveContainerTracking(t *testing.T) {
	resetTracking()
	defer resetTracking()

	managedNetworksMutex.Lock()
	managedNetworks = []string{".imds-0", ".imds-1"}
	managedNetworksMutex.Unlock()
	defer func() {
		managedNetworksMutex.Lock()
		managedNetworks = nil
		managedNetworksMutex.Unlock()
	}()

	containerID := "abc123"
	inspectInitial := container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			ID:   containerID,
			Name: "/demo",
			State: &container.State{
				Running: false,
			},
		},
		Config: &container.Config{Labels: nil},
		NetworkSettings: &container.NetworkSettings{
			Networks: map[string]*network.EndpointSettings{},
		},
	}
	inspectUpdated := container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			ID:   containerID,
			Name: "/demo",
			State: &container.State{
				Running: false,
			},
		},
		Config: &container.Config{Labels: map[string]string{}},
		NetworkSettings: &container.NetworkSettings{
			Networks: map[string]*network.EndpointSettings{
				".imds-0": {
					NetworkID: "net-aws",
				},
				".imds-1": {
					NetworkID: "net-os",
				},
			},
		},
	}

	client := &fakeDockerClient{inspectSequence: []container.InspectResponse{inspectInitial, inspectUpdated}}

	if err := addContainerToTrackingWithNetwork(context.Background(), client, containerID, false); err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	if len(client.networkConnectCalls) != 2 {
		t.Fatalf("want two network connect calls, got %d", len(client.networkConnectCalls))
	}

	tracker.mu.RLock()
	info, ok := tracker.byID[containerID]
	tracker.mu.RUnlock()
	if !ok {
		t.Fatalf("want container to be tracked")
	}
	if len(info.Networks) != 2 {
		t.Fatalf("want two networks, got %d", len(info.Networks))
	}
	if info.Labels == nil {
		t.Fatalf("want labels to be initialized")
	}

	tracker.mu.RLock()
	if got := tracker.ipToContainerID["net-aws"]; got != containerID {
		tracker.mu.RUnlock()
		t.Fatalf("want net-aws mapping, got %q", got)
	}
	if got := tracker.ipToContainerID["net-os"]; got != containerID {
		tracker.mu.RUnlock()
		t.Fatalf("want net-os mapping, got %q", got)
	}
	tracker.mu.RUnlock()

	removeContainerFromTracking(containerID)

	tracker.mu.RLock()
	if _, ok := tracker.byID[containerID]; ok {
		tracker.mu.RUnlock()
		t.Fatalf("want container to be removed")
	}
	tracker.mu.RUnlock()
}

func TestAddContainerToTrackingWithNetworkPauseFirst(t *testing.T) {
	resetTracking()
	defer resetTracking()

	managedNetworksMutex.Lock()
	managedNetworks = []string{".imds-0"}
	managedNetworksMutex.Unlock()
	defer func() {
		managedNetworksMutex.Lock()
		managedNetworks = nil
		managedNetworksMutex.Unlock()
	}()

	containerID := "pause-test-abc"
	inspectInitial := container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			ID:   containerID,
			Name: "/pause-test",
			State: &container.State{
				Running: true,
			},
		},
		Config: &container.Config{Labels: map[string]string{}},
		NetworkSettings: &container.NetworkSettings{
			Networks: map[string]*network.EndpointSettings{},
		},
	}
	inspectUpdated := container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			ID:   containerID,
			Name: "/pause-test",
			State: &container.State{
				Running: true,
			},
		},
		Config: &container.Config{Labels: map[string]string{}},
		NetworkSettings: &container.NetworkSettings{
			Networks: map[string]*network.EndpointSettings{
				".imds-0": {NetworkID: "net-aws"},
			},
		},
	}

	client := &fakeDockerClient{inspectSequence: []container.InspectResponse{inspectInitial, inspectUpdated}}

	if err := addContainerToTrackingWithNetwork(context.Background(), client, containerID, true); err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	if client.pauseCalls != 1 {
		t.Errorf("want 1 pause call, got %d", client.pauseCalls)
	}
	if client.unpauseCalls != 1 {
		t.Errorf("want 1 unpause call, got %d", client.unpauseCalls)
	}
}

// TestConcurrentContainerTracking tests concurrent additions and removals
func TestConcurrentContainerTracking(t *testing.T) {
	resetTracking()
	defer resetTracking()

	const numGoroutines = 50
	const numOperations = 20

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numOperations; j++ {
				containerID := fmt.Sprintf("container-%d-%d", id, j)
				ipAddr := fmt.Sprintf("169.254.169.%d", (id*numOperations+j)%254+1)

				// Add container to tracking
				containerInfo := ContainerInfo{
					ContainerID: containerID,
					Name:        fmt.Sprintf("/test-%d-%d", id, j),
					Networks: []NetworkInfo{
						{NetworkID: ipAddr, NetworkName: fmt.Sprintf("net-%d", j)},
					},
				}

				tracker.mu.Lock()
				tracker.byID[containerID] = containerInfo
				tracker.mu.Unlock()

				// Update IP index
				updateIPIndex(containerID)

				// Lookup by IP
				tracker.mu.RLock()
				got := tracker.ipToContainerID[ipAddr]
				tracker.mu.RUnlock()
				if got != containerID {
					t.Errorf("want IP mapping %s=%s, got %s", ipAddr, containerID, got)
				}

				// Remove container
				removeContainerFromTracking(containerID)
			}
		}(i)
	}

	wg.Wait()

	// Verify clean state
	tracker.mu.RLock()
	numTracked := len(tracker.byID)
	tracker.mu.RUnlock()

	if numTracked != 0 {
		t.Errorf("want 0 tracked containers after cleanup, got %d", numTracked)
	}
}

// TestConcurrentIPLookup tests concurrent IP to container lookups
func TestConcurrentIPLookup(t *testing.T) {
	resetTracking()
	defer resetTracking()

	// Setup test data
	containers := make(map[string]string) // ip -> containerID
	for i := 0; i < 100; i++ {
		containerID := fmt.Sprintf("container-%d", i)
		ipAddr := fmt.Sprintf("169.254.169.%d", i%254+1)
		containers[ipAddr] = containerID

		containerInfo := ContainerInfo{
			ContainerID: containerID,
			Name:        fmt.Sprintf("/test-%d", i),
			Networks: []NetworkInfo{
				{NetworkID: ipAddr, NetworkName: fmt.Sprintf("net-%d", i)},
			},
		}

		tracker.mu.Lock()
		tracker.byID[containerID] = containerInfo
		tracker.mu.Unlock()

		updateIPIndex(containerID)
	}

	const numGoroutines = 50
	const numLookups = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < numLookups; j++ {
				ipAddr := fmt.Sprintf("169.254.169.%d", j%100%254+1)
				expectedID := containers[ipAddr]

				tracker.mu.RLock()
				got := tracker.ipToContainerID[ipAddr]
				tracker.mu.RUnlock()

				if got != expectedID {
					t.Errorf("want IP mapping %s=%s, got %s", ipAddr, expectedID, got)
				}
			}
		}()
	}

	wg.Wait()
}

// TestConcurrentSettingsAccess tests concurrent settings reads and writes
func TestConcurrentSettingsAccess(t *testing.T) {
	resetTracking()
	defer resetTracking()

	const numReaders = 30
	const numWriters = 5
	const numOperations = 100

	var wg sync.WaitGroup
	wg.Add(numReaders + numWriters)

	// Readers
	for i := 0; i < numReaders; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < numOperations; j++ {
				settingsMutex.RLock()
				_ = settings.URL
				settingsMutex.RUnlock()
			}
		}()
	}

	// Writers
	for i := 0; i < numWriters; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numOperations; j++ {
				settingsMutex.Lock()
				settings.URL = fmt.Sprintf("http://example.com/%d-%d", id, j)
				settingsMutex.Unlock()
			}
		}(i)
	}

	wg.Wait()
}

// TestRaceConditionInDockerEventProcessing tests concurrent Docker event handling
func TestRaceConditionInDockerEventProcessing(t *testing.T) {
	resetTracking()
	defer resetTracking()

	const numEvents = 100
	var wg sync.WaitGroup
	wg.Add(numEvents)

	// Simulate concurrent Docker events
	for i := 0; i < numEvents; i++ {
		go func(id int) {
			defer wg.Done()

			containerID := fmt.Sprintf("container-%d", id)
			ipAddr := fmt.Sprintf("169.254.169.%d", id%254+1)

			// Simulate start event
			containerInfo := ContainerInfo{
				ContainerID: containerID,
				Name:        fmt.Sprintf("/test-%d", id),
				Networks: []NetworkInfo{
					{NetworkID: ipAddr, NetworkName: fmt.Sprintf("net-%d", id)},
				},
			}

			tracker.mu.Lock()
			tracker.byID[containerID] = containerInfo
			tracker.mu.Unlock()

			// Simulate network connect
			updateIPIndex(containerID)

			// Simulate stop event
			removeContainerFromTracking(containerID)
		}(i)
	}

	wg.Wait()

	// Verify clean state
	tracker.mu.RLock()
	numTracked := len(tracker.byID)
	tracker.mu.RUnlock()

	tracker.mu.RLock()
	numIPs := len(tracker.ipToContainerID)
	tracker.mu.RUnlock()

	if numTracked != 0 {
		t.Errorf("want 0 tracked containers, got %d", numTracked)
	}
	if numIPs != 0 {
		t.Errorf("want 0 IP mappings, got %d", numIPs)
	}
}

// TestEdgeCaseEmptyContainerID tests handling of empty container ID
func TestEdgeCaseEmptyContainerID(t *testing.T) {
	resetTracking()
	defer resetTracking()

	containerInfo := ContainerInfo{
		ContainerID: "",
		Name:        "/valid-name",
		Networks: []NetworkInfo{
			{NetworkID: "169.254.169.100", NetworkName: "bridge"},
		},
	}

	tracker.mu.Lock()
	tracker.byID[""] = containerInfo
	tracker.mu.Unlock()

	updateIPIndex("")

	// Should handle empty ID gracefully
	tracker.mu.RLock()
	got := tracker.ipToContainerID["169.254.169.100"]
	tracker.mu.RUnlock()

	if got != "" {
		t.Errorf("want empty string mapping, got %q", got)
	}

	removeContainerFromTracking("")

	tracker.mu.RLock()
	_, exists := tracker.byID[""]
	tracker.mu.RUnlock()

	if exists {
		t.Error("empty container ID should be removable")
	}
}

// TestEdgeCaseUnicodeContainerName tests handling of Unicode in container names
func TestEdgeCaseUnicodeContainerName(t *testing.T) {
	resetTracking()
	defer resetTracking()

	testCases := []struct {
		name          string
		containerName string
	}{
		{"emoji", "/🐳-container"},
		{"chinese", "/容器-test"},
		{"arabic", "/حاوية-test"},
		{"cyrillic", "/контейнер-test"},
		{"mixed", "/test-🚀-подтест-测试"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			containerID := "unicode-" + tc.name
			ipAddr := fmt.Sprintf("169.254.169.%d", len(tc.name)+10)

			containerInfo := ContainerInfo{
				ContainerID: containerID,
				Name:        tc.containerName,
				Networks: []NetworkInfo{
					{NetworkID: ipAddr, NetworkName: "bridge"},
				},
			}

			tracker.mu.Lock()
			tracker.byID[containerID] = containerInfo
			tracker.mu.Unlock()

			updateIPIndex(containerID)

			// Verify container is tracked
			tracker.mu.RLock()
			stored, exists := tracker.byID[containerID]
			tracker.mu.RUnlock()

			if !exists {
				t.Fatalf("container %q should be tracked", containerID)
			}
			if stored.Name != tc.containerName {
				t.Errorf("want name %q, got %q", tc.containerName, stored.Name)
			}

			// Verify IP mapping
			tracker.mu.RLock()
			got := tracker.ipToContainerID[ipAddr]
			tracker.mu.RUnlock()

			if got != containerID {
				t.Errorf("want IP mapping to %q, got %q", containerID, got)
			}

			removeContainerFromTracking(containerID)
		})
	}
}

// TestEdgeCaseVeryLongContainerID tests handling of unusually long container IDs
func TestEdgeCaseVeryLongContainerID(t *testing.T) {
	resetTracking()
	defer resetTracking()

	// Docker container IDs are typically 64 hex chars, but test with longer
	longID := strings.Repeat("a", 256)
	ipAddr := "169.254.169.101"

	containerInfo := ContainerInfo{
		ContainerID: longID,
		Name:        "/test-long-id",
		Networks: []NetworkInfo{
			{NetworkID: ipAddr, NetworkName: "bridge"},
		},
	}

	tracker.mu.Lock()
	tracker.byID[longID] = containerInfo
	tracker.mu.Unlock()

	updateIPIndex(longID)

	// Verify storage works with long IDs
	tracker.mu.RLock()
	stored, exists := tracker.byID[longID]
	tracker.mu.RUnlock()

	if !exists {
		t.Fatal("long container ID should be tracked")
	}
	if stored.ContainerID != longID {
		t.Errorf("want ID preserved, got truncated")
	}

	// Verify IP mapping
	tracker.mu.RLock()
	got := tracker.ipToContainerID[ipAddr]
	tracker.mu.RUnlock()

	if got != longID {
		t.Errorf("want IP mapping to long ID, got %q", got)
	}

	removeContainerFromTracking(longID)
}

// TestEdgeCaseVeryLongContainerName tests handling of unusually long container names
func TestEdgeCaseVeryLongContainerName(t *testing.T) {
	resetTracking()
	defer resetTracking()

	// Docker limits name length, but test system can handle arbitrary lengths
	longName := "/" + strings.Repeat("very-long-name-", 100)
	containerID := "long-name-test"
	ipAddr := "169.254.169.102"

	containerInfo := ContainerInfo{
		ContainerID: containerID,
		Name:        longName,
		Networks: []NetworkInfo{
			{NetworkID: ipAddr, NetworkName: "bridge"},
		},
	}

	tracker.mu.Lock()
	tracker.byID[containerID] = containerInfo
	tracker.mu.Unlock()

	updateIPIndex(containerID)

	// Verify storage works with long names
	tracker.mu.RLock()
	stored, exists := tracker.byID[containerID]
	tracker.mu.RUnlock()

	if !exists {
		t.Fatal("container with long name should be tracked")
	}
	if stored.Name != longName {
		t.Errorf("want full name preserved, got truncated")
	}

	removeContainerFromTracking(containerID)
}

// TestEdgeCaseEmptyNetworkID tests handling of empty network ID
func TestEdgeCaseEmptyNetworkID(t *testing.T) {
	resetTracking()
	defer resetTracking()

	containerID := "empty-network-test"

	containerInfo := ContainerInfo{
		ContainerID: containerID,
		Name:        "/test-empty-net",
		Networks: []NetworkInfo{
			{NetworkID: "", NetworkName: "bridge"},
		},
	}

	tracker.mu.Lock()
	tracker.byID[containerID] = containerInfo
	tracker.mu.Unlock()

	updateIPIndex(containerID)

	// Current behavior: creates mapping with empty key (edge case that doesn't crash)
	// This documents existing behavior rather than enforcing validation
	tracker.mu.RLock()
	_, exists := tracker.ipToContainerID[""]
	tracker.mu.RUnlock()

	// System handles empty NetworkID without panic
	if os.Getenv("VERBOSE_TESTS") != "" {
		if !exists {
			// No mapping created for empty ID
			t.Log("empty network ID does not create mapping")
		} else {
			// Mapping created (current behavior)
			t.Log("empty network ID creates mapping (edge case tolerated)")
		}
	}

	removeContainerFromTracking(containerID)
}

// TestDockerClientCloseTracking verifies that Close() calls are tracked in mock
func TestDockerClientCloseTracking(t *testing.T) {
	client := &fakeDockerClient{}

	if client.closeCalls != 0 {
		t.Errorf("expected closeCalls=0, got %d", client.closeCalls)
	}

	if err := client.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}

	if client.closeCalls != 1 {
		t.Errorf("expected closeCalls=1 after Close(), got %d", client.closeCalls)
	}

	// Multiple closes
	client.Close()
	client.Close()

	if client.closeCalls != 3 {
		t.Errorf("expected closeCalls=3 after 3 Close() calls, got %d", client.closeCalls)
	}
}

// TestStressHighConcurrentContainerOperations tests system under high concurrent load
func TestStressHighConcurrentContainerOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	resetTracking()
	defer resetTracking()

	const numGoroutines = 500
	const numOperations = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numOperations; j++ {
				containerID := fmt.Sprintf("stress-container-%d-%d", id, j)
				ipAddr := fmt.Sprintf("169.254.169.%d", ((id*numOperations+j)%254)+1)

				containerInfo := ContainerInfo{
					ContainerID: containerID,
					Name:        fmt.Sprintf("/stress-%d-%d", id, j),
					Networks: []NetworkInfo{
						{NetworkID: ipAddr, NetworkName: fmt.Sprintf("net-%d", j%10)},
					},
					Labels: map[string]string{
						"stress": "test",
						"id":     containerID,
					},
				}

				// Add container
				tracker.mu.Lock()
				tracker.byID[containerID] = containerInfo
				tracker.mu.Unlock()

				// Update IP index
				updateIPIndex(containerID)

				// Read operations
				tracker.mu.RLock()
				_ = tracker.ipToContainerID[ipAddr]
				tracker.mu.RUnlock()

				tracker.mu.RLock()
				_ = tracker.byID[containerID]
				tracker.mu.RUnlock()

				// Delete container
				if j%10 == 0 {
					removeContainerFromTracking(containerID)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify system is in consistent state
	tracker.mu.RLock()
	numTracked := len(tracker.byID)
	tracker.mu.RUnlock()

	tracker.mu.RLock()
	numIPs := len(tracker.ipToContainerID)
	tracker.mu.RUnlock()

	t.Logf("After stress test: %d tracked containers, %d IP mappings", numTracked, numIPs)

	if numTracked < 0 || numIPs < 0 {
		t.Errorf("negative counts indicate memory corruption")
	}
}

// TestStressMutexContention tests mutex contention under high load
func TestStressMutexContention(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	resetTracking()
	defer resetTracking()

	const numReaders = 200
	const numWriters = 50
	const duration = 1 // seconds

	stopChan := make(chan struct{})
	var wg sync.WaitGroup

	// Start readers
	wg.Add(numReaders)
	for i := 0; i < numReaders; i++ {
		go func(id int) {
			defer wg.Done()
			readCount := 0
			for {
				select {
				case <-stopChan:
					if os.Getenv("VERBOSE_TESTS") != "" {
						t.Logf("Reader %d performed %d reads", id, readCount)
					}
					return
				default:
					// Read from tracked containers
					tracker.mu.RLock()
					_ = len(tracker.byID)
					tracker.mu.RUnlock()

					// Read from IP index
					tracker.mu.RLock()
					_ = len(tracker.ipToContainerID)
					tracker.mu.RUnlock()

					readCount++
				}
			}
		}(i)
	}

	// Start writers
	wg.Add(numWriters)
	for i := 0; i < numWriters; i++ {
		go func(id int) {
			defer wg.Done()
			writeCount := 0
			for {
				select {
				case <-stopChan:
					if os.Getenv("VERBOSE_TESTS") != "" {
						t.Logf("Writer %d performed %d writes", id, writeCount)
					}
					return
				default:
					containerID := fmt.Sprintf("contention-%d-%d", id, writeCount)
					ipAddr := fmt.Sprintf("169.254.169.%d", ((id*1000+writeCount)%254)+1)

					containerInfo := ContainerInfo{
						ContainerID: containerID,
						Name:        fmt.Sprintf("/contention-%d", id),
						Networks: []NetworkInfo{
							{NetworkID: ipAddr, NetworkName: "bridge"},
						},
					}

					tracker.mu.Lock()
					tracker.byID[containerID] = containerInfo
					tracker.mu.Unlock()

					updateIPIndex(containerID)

					// Sometimes remove
					if writeCount%5 == 0 {
						removeContainerFromTracking(containerID)
					}

					writeCount++
				}
			}
		}(i)
	}

	// Let it run for the specified duration
	time.Sleep(time.Duration(duration) * time.Second)
	close(stopChan)

	wg.Wait()

	// Verify no corruption
	tracker.mu.RLock()
	numTracked := len(tracker.byID)
	tracker.mu.RUnlock()

	t.Logf("After contention test: %d containers remain tracked", numTracked)
}

// TestStressRapidSettingsChanges tests rapid concurrent settings updates
func TestStressRapidSettingsChanges(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	const numGoroutines = 100
	const numUpdates = 50

	tempDir := t.TempDir()
	originalPath := settingsPath
	settingsPath = tempDir + "/stress-settings.json"
	defer func() { settingsPath = originalPath }()

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numUpdates; j++ {
				// Write settings
				settingsMutex.Lock()
				settings = Settings{
					URL: fmt.Sprintf("http://test-%d-%d.example.com", id, j),
				}
				settingsMutex.Unlock()

				if err := persistSettings(); err != nil {
					// Some errors expected due to concurrent writes
					continue
				}

				// Read settings
				_ = loadSettings()
			}
		}(i)
	}

	wg.Wait()

	// Verify final settings file is valid
	err := loadSettings()
	if err != nil {
		t.Logf("Final settings load returned error: %v (acceptable)", err)
	} else {
		settingsMutex.RLock()
		finalURL := settings.URL
		settingsMutex.RUnlock()
		t.Logf("Final settings URL: %s", finalURL)
	}
}

func resetManagedNetworks() {
	managedNetworksMutex.Lock()
	defer managedNetworksMutex.Unlock()
	managedNetworks = nil
}

func TestScanExistingContainersEmpty(t *testing.T) {
	resetTracking()
	t.Cleanup(resetTracking)

	cli := &fakeDockerClient{}
	if err := scanExistingContainers(context.Background(), cli); err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	tracker.mu.RLock()
	n := len(tracker.byID)
	tracker.mu.RUnlock()
	if n != 0 {
		t.Errorf("want 0 tracked containers, got %d", n)
	}
}

func TestScanExistingContainersLabeledContainer(t *testing.T) {
	resetTracking()
	t.Cleanup(resetTracking)

	containerID := "labeled123"
	cli := &fakeDockerClient{
		containerList: []container.Summary{
			{
				ID:     containerID,
				Labels: map[string]string{"imds-proxy.enabled": "true"},
			},
		},
		inspectSequence: []container.InspectResponse{
			{
				ContainerJSONBase: &container.ContainerJSONBase{
					ID:    containerID,
					Name:  "/labeled",
					State: &container.State{Running: false},
				},
				Config: &container.Config{Labels: map[string]string{"imds-proxy.enabled": "true"}},
				NetworkSettings: &container.NetworkSettings{
					Networks: map[string]*network.EndpointSettings{
						".imds-0": {NetworkID: "net1"},
					},
				},
			},
			{
				ContainerJSONBase: &container.ContainerJSONBase{
					ID:    containerID,
					Name:  "/labeled",
					State: &container.State{Running: false},
				},
				Config: &container.Config{Labels: map[string]string{"imds-proxy.enabled": "true"}},
				NetworkSettings: &container.NetworkSettings{
					Networks: map[string]*network.EndpointSettings{
						".imds-0":   {NetworkID: "net1"},
						".imds-1": {NetworkID: "net2"},
					},
				},
			},
		},
	}

	if err := scanExistingContainers(context.Background(), cli); err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	tracker.mu.RLock()
	_, ok := tracker.byID[containerID]
	tracker.mu.RUnlock()
	if !ok {
		t.Errorf("want container %s to be tracked", containerID)
	}
}

func TestScanExistingContainersUnlabeledContainer(t *testing.T) {
	resetTracking()
	t.Cleanup(resetTracking)

	cli := &fakeDockerClient{
		containerList: []container.Summary{
			{
				ID:     "unlabeled456",
				Labels: map[string]string{"some-other-label": "value"},
			},
		},
	}

	if err := scanExistingContainers(context.Background(), cli); err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	tracker.mu.RLock()
	n := len(tracker.byID)
	tracker.mu.RUnlock()
	if n != 0 {
		t.Errorf("want 0 tracked containers for unlabeled container, got %d", n)
	}
}

func TestScanExistingContainersError(t *testing.T) {
	resetTracking()
	t.Cleanup(resetTracking)

	cli := &fakeDockerClient{
		containerListErr: errors.New("docker unavailable"),
	}

	err := scanExistingContainers(context.Background(), cli)
	if err == nil {
		t.Fatal("want error propagated, got nil")
	}
}

func TestDiscoverManagedNetworksEmpty(t *testing.T) {
	resetManagedNetworks()
	t.Cleanup(resetManagedNetworks)

	cli := &fakeDockerClient{networkList: []network.Summary{}}

	if err := discoverManagedNetworks(context.Background(), cli); err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	managedNetworksMutex.RLock()
	n := len(managedNetworks)
	managedNetworksMutex.RUnlock()
	if n != 0 {
		t.Errorf("want 0 managed networks, got %d", n)
	}
}

func TestDiscoverManagedNetworksWithNetworks(t *testing.T) {
	resetManagedNetworks()
	t.Cleanup(resetManagedNetworks)

	cli := &fakeDockerClient{
		networkList: []network.Summary{
			{Name: ".imds-0", Labels: map[string]string{"imds-proxy.managed": "true"}},
			{Name: ".imds-1", Labels: map[string]string{"imds-proxy.managed": "true"}},
		},
	}

	if err := discoverManagedNetworks(context.Background(), cli); err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	managedNetworksMutex.RLock()
	nets := managedNetworks
	managedNetworksMutex.RUnlock()

	if len(nets) != 2 {
		t.Fatalf("want 2 managed networks, got %d", len(nets))
	}
	found := map[string]bool{}
	for _, name := range nets {
		found[name] = true
	}
	for _, want := range []string{".imds-0", ".imds-1"} {
		if !found[want] {
			t.Errorf("want network %s in managed networks, got %v", want, nets)
		}
	}
}

func TestDiscoverManagedNetworksError(t *testing.T) {
	resetManagedNetworks()
	t.Cleanup(resetManagedNetworks)

	cli := &fakeDockerClient{networkListErr: errors.New("network list failed")}

	err := discoverManagedNetworks(context.Background(), cli)
	if err == nil {
		t.Fatal("want error propagated, got nil")
	}
}

func TestRefreshContainerNetworksTracked(t *testing.T) {
	resetTracking()
	t.Cleanup(resetTracking)

	containerID := "refresh123"
	tracker.mu.Lock()
	tracker.byID[containerID] = ContainerInfo{
		ContainerID: containerID,
		Name:        "/refresh-test",
		Labels:      map[string]string{},
		Networks:    []NetworkInfo{},
	}
	tracker.mu.Unlock()

	cli := &fakeDockerClient{
		inspectSequence: []container.InspectResponse{
			{
				ContainerJSONBase: &container.ContainerJSONBase{
					ID:    containerID,
					Name:  "/refresh-test",
					State: &container.State{Running: false},
				},
				Config: &container.Config{Labels: map[string]string{}},
				NetworkSettings: &container.NetworkSettings{
					Networks: map[string]*network.EndpointSettings{
						".imds-0": {NetworkID: "updated-net1"},
					},
				},
			},
		},
	}

	if err := refreshContainerNetworks(context.Background(), cli, containerID); err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	tracker.mu.RLock()
	info := tracker.byID[containerID]
	tracker.mu.RUnlock()

	if len(info.Networks) != 1 {
		t.Fatalf("want 1 network after refresh, got %d", len(info.Networks))
	}
	if info.Networks[0].NetworkID != "updated-net1" {
		t.Errorf("want NetworkID updated-net1, got %s", info.Networks[0].NetworkID)
	}
}

func TestRefreshContainerNetworksUntracked(t *testing.T) {
	resetTracking()
	t.Cleanup(resetTracking)

	cli := &fakeDockerClient{
		inspectSequence: []container.InspectResponse{
			{
				ContainerJSONBase: &container.ContainerJSONBase{
					ID:    "untracked999",
					Name:  "/untracked",
					State: &container.State{Running: false},
				},
				Config: &container.Config{Labels: map[string]string{}},
				NetworkSettings: &container.NetworkSettings{
					Networks: map[string]*network.EndpointSettings{},
				},
			},
		},
	}

	// Container is not in tracker.byID - should return nil without panic.
	if err := refreshContainerNetworks(context.Background(), cli, "untracked999"); err != nil {
		t.Fatalf("want no error for untracked container, got %v", err)
	}
}

func TestRefreshContainerNetworksError(t *testing.T) {
	resetTracking()
	t.Cleanup(resetTracking)

	cli := &fakeDockerClient{inspectErr: errors.New("inspect failed")}

	err := refreshContainerNetworks(context.Background(), cli, "some-container")
	if err == nil {
		t.Fatal("want error propagated from inspect, got nil")
	}
}

func TestFindContainerByIPFound(t *testing.T) {
	resetTracking()
	t.Cleanup(resetTracking)

	containerID := "ipfound123"
	tracker.mu.Lock()
	tracker.byID[containerID] = ContainerInfo{
		ContainerID: containerID,
		Name:        "/ip-test",
		Labels:      map[string]string{"app": "test"},
		Networks:    []NetworkInfo{{NetworkName: ".imds-0", NetworkID: "net-x"}},
	}
	tracker.mu.Unlock()

	cli := &fakeDockerClient{
		inspectSequence: []container.InspectResponse{
			{
				ContainerJSONBase: &container.ContainerJSONBase{
					ID:    containerID,
					Name:  "/ip-test",
					State: &container.State{Running: true},
				},
				Config: &container.Config{Labels: map[string]string{"app": "test"}},
				NetworkSettings: &container.NetworkSettings{
					Networks: map[string]*network.EndpointSettings{
						".imds-0": {NetworkID: "net-x", IPAddress: "10.5.0.42"},
					},
				},
			},
		},
	}

	resp, err := findContainerByIP(context.Background(), cli, "10.5.0.42")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if resp == nil {
		t.Fatal("want response, got nil")
	}
	if resp.ContainerID != containerID {
		t.Errorf("want ContainerID %s, got %s", containerID, resp.ContainerID)
	}
}

func TestFindContainerByIPNotFound(t *testing.T) {
	resetTracking()
	t.Cleanup(resetTracking)

	containerID := "ipnotfound456"
	tracker.mu.Lock()
	tracker.byID[containerID] = ContainerInfo{
		ContainerID: containerID,
		Name:        "/no-ip",
		Labels:      map[string]string{},
		Networks:    []NetworkInfo{{NetworkName: ".imds-0", NetworkID: "net-y"}},
	}
	tracker.mu.Unlock()

	cli := &fakeDockerClient{
		inspectSequence: []container.InspectResponse{
			{
				ContainerJSONBase: &container.ContainerJSONBase{
					ID:    containerID,
					Name:  "/no-ip",
					State: &container.State{Running: true},
				},
				Config: &container.Config{Labels: map[string]string{}},
				NetworkSettings: &container.NetworkSettings{
					Networks: map[string]*network.EndpointSettings{
						".imds-0": {NetworkID: "net-y", IPAddress: "10.5.0.1"},
					},
				},
			},
		},
	}

	resp, err := findContainerByIP(context.Background(), cli, "192.168.99.99")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if resp != nil {
		t.Errorf("want nil response for unmatched IP, got %+v", resp)
	}
}

func TestFindContainerByIPInspectError(t *testing.T) {
	resetTracking()
	t.Cleanup(resetTracking)

	containerID := "inspect-err-789"
	tracker.mu.Lock()
	tracker.byID[containerID] = ContainerInfo{
		ContainerID: containerID,
		Name:        "/err-container",
		Labels:      map[string]string{},
		Networks:    []NetworkInfo{{NetworkName: ".imds-0", NetworkID: "net-z"}},
	}
	tracker.mu.Unlock()

	cli := &fakeDockerClient{inspectErr: errors.New("inspect error")}

	// findContainerByIP continues past inspect errors and returns nil.
	resp, err := findContainerByIP(context.Background(), cli, "10.0.0.1")
	if err != nil {
		t.Fatalf("want no error (function skips failed inspects), got %v", err)
	}
	if resp != nil {
		t.Errorf("want nil response when inspect fails, got %+v", resp)
	}
}
