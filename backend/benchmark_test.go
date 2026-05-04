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
	"fmt"
	"testing"
)

// BenchmarkContainerTracking measures performance of container add/remove operations
func BenchmarkContainerTracking(b *testing.B) {
	resetTracking()
	defer resetTracking()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		containerID := fmt.Sprintf("bench-container-%d", i)
		ipAddr := fmt.Sprintf("169.254.169.%d", (i%254)+1)

		containerInfo := ContainerInfo{
			ContainerID: containerID,
			Name:        fmt.Sprintf("/bench-%d", i),
			Networks: []NetworkInfo{
				{NetworkID: ipAddr, NetworkName: "bridge"},
			},
			Labels: map[string]string{
				"test": "benchmark",
			},
		}

		tracker.mu.Lock()
		tracker.byID[containerID] = containerInfo
		tracker.mu.Unlock()

		updateIPIndex(containerID)

		if i%10 == 0 {
			removeContainerFromTracking(containerID)
		}
	}
}

// BenchmarkIPLookup measures performance of IP to container ID lookups
func BenchmarkIPLookup(b *testing.B) {
	resetTracking()
	defer resetTracking()

	// Pre-populate with containers
	for i := 0; i < 100; i++ {
		containerID := fmt.Sprintf("container-%d", i)
		ipAddr := fmt.Sprintf("169.254.169.%d", (i%254)+1)

		containerInfo := ContainerInfo{
			ContainerID: containerID,
			Name:        fmt.Sprintf("/test-%d", i),
			Networks: []NetworkInfo{
				{NetworkID: ipAddr, NetworkName: "bridge"},
			},
		}

		tracker.mu.Lock()
		tracker.byID[containerID] = containerInfo
		tracker.mu.Unlock()

		updateIPIndex(containerID)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ipAddr := fmt.Sprintf("169.254.169.%d", (i%100)%254+1)

		tracker.mu.RLock()
		_ = tracker.ipToContainerID[ipAddr]
		tracker.mu.RUnlock()
	}
}

// BenchmarkSettingsAccess measures performance of settings read operations
func BenchmarkSettingsAccess(b *testing.B) {
	settings.Set(Settings{URL: "http://benchmark.example.com"})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = settings.Get().URL
	}
}

// BenchmarkConcurrentReads measures read performance under concurrent access
func BenchmarkConcurrentReads(b *testing.B) {
	resetTracking()
	defer resetTracking()

	// Pre-populate with containers
	for i := 0; i < 100; i++ {
		containerID := fmt.Sprintf("concurrent-%d", i)
		ipAddr := fmt.Sprintf("169.254.169.%d", (i%254)+1)

		containerInfo := ContainerInfo{
			ContainerID: containerID,
			Name:        fmt.Sprintf("/concurrent-%d", i),
			Networks: []NetworkInfo{
				{NetworkID: ipAddr, NetworkName: "bridge"},
			},
		}

		tracker.mu.Lock()
		tracker.byID[containerID] = containerInfo
		tracker.mu.Unlock()

		updateIPIndex(containerID)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			ipAddr := fmt.Sprintf("169.254.169.%d", (i%100)%254+1)

			tracker.mu.RLock()
			_ = tracker.ipToContainerID[ipAddr]
			tracker.mu.RUnlock()

			i++
		}
	})
}
