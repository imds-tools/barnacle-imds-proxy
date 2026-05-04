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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/labstack/echo/v4"
)

func withNoOpNotify(t *testing.T) {
	t.Helper()
	old := notifyProxyConfigUpdateFn
	notifyProxyConfigUpdateFn = func() {}
	t.Cleanup(func() { notifyProxyConfigUpdateFn = old })

	oldReconcile := reconcileNetworksFn
	reconcileNetworksFn = func(_ context.Context, _ DockerClient, _ []NetworkConfig) error { return nil }
	t.Cleanup(func() { reconcileNetworksFn = oldReconcile })
}

func withTempSettingsPath(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	old := settingsPath
	settingsPath = filepath.Join(dir, "settings.json")
	t.Cleanup(func() { settingsPath = old })
}

func withSettings(t *testing.T, s Settings) {
	t.Helper()
	settingsMutex.Lock()
	settings = s
	settingsMutex.Unlock()
	t.Cleanup(func() {
		settingsMutex.Lock()
		settings = Settings{}
		settingsMutex.Unlock()
	})
}

func TestHello(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := hello(c); err != nil {
		t.Fatalf("hello() returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("want status 200, got %d", rec.Code)
	}
}

func TestGetSettings(t *testing.T) {
	withSettings(t, Settings{URL: "http://test.example.com"})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := getSettings(c); err != nil {
		t.Fatalf("getSettings() returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("want status 200, got %d", rec.Code)
	}

	var got Settings
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.URL != "http://test.example.com" {
		t.Errorf("want URL %q, got %q", "http://test.example.com", got.URL)
	}
}

func TestSaveSettingsValid(t *testing.T) {
	withTempSettingsPath(t)
	withSettings(t, Settings{})
	withNoOpNotify(t)

	e := echo.New()
	body := strings.NewReader(`{"url":"http://example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/settings", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := saveSettings(c); err != nil {
		t.Fatalf("saveSettings() returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("want status 200, got %d", rec.Code)
	}

	settingsMutex.RLock()
	gotURL := settings.URL
	settingsMutex.RUnlock()
	if gotURL != "http://example.com" {
		t.Errorf("want settings URL %q, got %q", "http://example.com", gotURL)
	}
}

func TestSaveSettingsEmptyURL(t *testing.T) {
	withTempSettingsPath(t)

	e := echo.New()
	body := strings.NewReader(`{"url":""}`)
	req := httptest.NewRequest(http.MethodPost, "/settings", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := saveSettings(c); err != nil {
		t.Fatalf("saveSettings() returned error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want status 400, got %d", rec.Code)
	}
}

func TestSaveSettingsInvalidJSON(t *testing.T) {
	withTempSettingsPath(t)

	e := echo.New()
	body := strings.NewReader(`not-json`)
	req := httptest.NewRequest(http.MethodPost, "/settings", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := saveSettings(c); err != nil {
		t.Fatalf("saveSettings() returned error: %v", err)
	}

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want status 400, got %d", rec.Code)
	}
}

func TestSaveSettingsWithCustomIPs(t *testing.T) {
	withTempSettingsPath(t)
	withSettings(t, Settings{})
	withNoOpNotify(t)

	e := echo.New()
	body := strings.NewReader(`{"url":"http://example.com","customIPs":["169.254.169.254","fd00:ec2::254"]}`)
	req := httptest.NewRequest(http.MethodPost, "/settings", body)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := saveSettings(c); err != nil {
		t.Fatalf("saveSettings() returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("want status 200, got %d", rec.Code)
	}

	settingsMutex.RLock()
	nets := settings.NetworkConfig
	settingsMutex.RUnlock()

	if len(nets) != 2 {
		t.Fatalf("want 2 network configs, got %d", len(nets))
	}
	if nets[0].ProxyIPv4 != "169.254.169.254" {
		t.Errorf("want nets[0].ProxyIPv4 169.254.169.254, got %q", nets[0].ProxyIPv4)
	}
	if nets[0].Name != ".imds-169.254.169.0" {
		t.Errorf("want nets[0].Name .imds-169.254.169.0, got %q", nets[0].Name)
	}
	if nets[1].ProxyIPv6 != "fd00:ec2::254" {
		t.Errorf("want nets[1].ProxyIPv6 fd00:ec2::254, got %q", nets[1].ProxyIPv6)
	}
	if nets[1].Name != ".imds-fd00-ec2--" {
		t.Errorf("want nets[1].Name .imds-fd00-ec2--, got %q", nets[1].Name)
	}
}

func TestComputeNetworkConfigEmpty(t *testing.T) {
	if nets := computeNetworkConfig([]string{}); len(nets) != 0 {
		t.Fatalf("want 0 networks, got %d", len(nets))
	}
}

func TestComputeNetworkConfigIPv4Only(t *testing.T) {
	nets := computeNetworkConfig([]string{"169.254.169.254"})
	if len(nets) != 1 {
		t.Fatalf("want 1 network, got %d", len(nets))
	}
	if nets[0].ProxyIPv4 != "169.254.169.254" {
		t.Errorf("want ProxyIPv4 169.254.169.254, got %q", nets[0].ProxyIPv4)
	}
	if nets[0].IPv6Subnet != "" {
		t.Errorf("want no IPv6 subnet, got %q", nets[0].IPv6Subnet)
	}
}

func TestComputeNetworkConfigIPv6Only(t *testing.T) {
	nets := computeNetworkConfig([]string{"fd00:ec2::254"})
	if len(nets) != 1 {
		t.Fatalf("want 1 network, got %d", len(nets))
	}
	if nets[0].ProxyIPv6 != "fd00:ec2::254" {
		t.Errorf("want ProxyIPv6 fd00:ec2::254, got %q", nets[0].ProxyIPv6)
	}
	if nets[0].IPv4Subnet != "" {
		t.Errorf("want no IPv4 subnet, got %q", nets[0].IPv4Subnet)
	}
}

func TestComputeNetworkConfigInvalidIPSkipped(t *testing.T) {
	nets := computeNetworkConfig([]string{"not-an-ip", "169.254.169.254"})
	if len(nets) != 1 {
		t.Fatalf("want 1 network (invalid IP skipped), got %d", len(nets))
	}
}

func TestComputeNetworkConfigDeduplicatesSubnet(t *testing.T) {
	nets := computeNetworkConfig([]string{"169.254.169.254", "169.254.169.1"})
	if len(nets) != 1 {
		t.Fatalf("want 1 network for same /24, got %d", len(nets))
	}
}

func TestReconcileNetworksCreatesNew(t *testing.T) {
	managedNetworksMutex.Lock()
	managedNetworks = nil
	managedNetworksMutex.Unlock()
	t.Cleanup(func() {
		managedNetworksMutex.Lock()
		managedNetworks = nil
		managedNetworksMutex.Unlock()
	})

	cli := &fakeDockerClient{networkList: []network.Summary{}}
	desired := []NetworkConfig{{Name: ".imds-0", IPv4Subnet: "169.254.169.0/24", ProxyIPv4: "169.254.169.254"}}

	if err := reconcileNetworks(context.Background(), cli, desired); err != nil {
		t.Fatalf("reconcileNetworks() error: %v", err)
	}
	if len(cli.networkCreateCalls) != 1 || cli.networkCreateCalls[0] != ".imds-0" {
		t.Errorf("want networkCreate .imds-0, got %v", cli.networkCreateCalls)
	}
}

func TestReconcileNetworksRemovesUnneeded(t *testing.T) {
	managedNetworksMutex.Lock()
	managedNetworks = nil
	managedNetworksMutex.Unlock()
	t.Cleanup(func() {
		managedNetworksMutex.Lock()
		managedNetworks = nil
		managedNetworksMutex.Unlock()
	})

	cli := &fakeDockerClient{
		networkList: []network.Summary{
			{ID: "net-abc", Name: ".imds-0", Labels: map[string]string{imdsManagedLabel: "true"}},
		},
	}

	if err := reconcileNetworks(context.Background(), cli, []NetworkConfig{}); err != nil {
		t.Fatalf("reconcileNetworks() error: %v", err)
	}
	if len(cli.networkRemoveCalls) != 1 || cli.networkRemoveCalls[0] != "net-abc" {
		t.Errorf("want networkRemove net-abc, got %v", cli.networkRemoveCalls)
	}
}

func TestReconcileNetworksRemovesOnSubnetChange(t *testing.T) {
	managedNetworksMutex.Lock()
	managedNetworks = nil
	managedNetworksMutex.Unlock()
	t.Cleanup(func() {
		managedNetworksMutex.Lock()
		managedNetworks = nil
		managedNetworksMutex.Unlock()
	})

	cli := &fakeDockerClient{
		networkList: []network.Summary{
			{
				ID:     "net-old",
				Name:   ".imds-0",
				Labels: map[string]string{imdsManagedLabel: "true"},
				IPAM: network.IPAM{
					Config: []network.IPAMConfig{{Subnet: "169.254.169.0/24"}},
				},
			},
		},
	}
	desired := []NetworkConfig{
		{Name: ".imds-0", IPv4Subnet: "10.0.0.0/24", ProxyIPv4: "10.0.0.1"},
	}

	if err := reconcileNetworks(context.Background(), cli, desired); err != nil {
		t.Fatalf("reconcileNetworks() error: %v", err)
	}
	if len(cli.networkRemoveCalls) != 1 || cli.networkRemoveCalls[0] != "net-old" {
		t.Errorf("want networkRemove net-old, got %v", cli.networkRemoveCalls)
	}
	if len(cli.networkCreateCalls) != 1 || cli.networkCreateCalls[0] != ".imds-0" {
		t.Errorf("want networkCreate .imds-0, got %v", cli.networkCreateCalls)
	}
}

func TestReconcileNetworksListError(t *testing.T) {
	cli := &fakeDockerClient{networkListErr: errors.New("docker unavailable")}
	if err := reconcileNetworks(context.Background(), cli, nil); err == nil {
		t.Fatal("want error when NetworkList fails, got nil")
	}
}

func TestDisconnectAllFromNetwork(t *testing.T) {
	cli := &fakeDockerClient{
		containerList: []container.Summary{
			{
				ID: "ctr1",
				NetworkSettings: &container.NetworkSettingsSummary{
					Networks: map[string]*network.EndpointSettings{".imds-0": {}},
				},
			},
		},
	}

	if err := disconnectAllFromNetwork(context.Background(), cli, ".imds-0"); err != nil {
		t.Fatalf("disconnectAllFromNetwork() error: %v", err)
	}
	if len(cli.networkDisconnectCalls) != 1 || cli.networkDisconnectCalls[0] != ".imds-0:ctr1" {
		t.Errorf("want disconnect .imds-0:ctr1, got %v", cli.networkDisconnectCalls)
	}
}

func TestDisconnectAllFromNetworkListError(t *testing.T) {
	cli := &fakeDockerClient{containerListErr: errors.New("list failed")}
	if err := disconnectAllFromNetwork(context.Background(), cli, ".imds-0"); err == nil {
		t.Fatal("want error when ContainerList fails, got nil")
	}
}

func TestGetContainersEmpty(t *testing.T) {
	resetTracking()
	t.Cleanup(resetTracking)
	withDockerClient(t, &fakeDockerClient{})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/containers", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := getContainers(c); err != nil {
		t.Fatalf("getContainers() returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("want status 200, got %d", rec.Code)
	}

	var result ContainersAPIResponse
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(result.Containers) != 0 {
		t.Errorf("want empty containers, got %d items", len(result.Containers))
	}
}

func TestGetContainersWithData(t *testing.T) {
	resetTracking()
	t.Cleanup(resetTracking)
	withDockerClient(t, &fakeDockerClient{})

	tracker.mu.Lock()
	tracker.byID["abc"] = ContainerInfo{
		ContainerID: "abc",
		Name:        "/test",
		Labels:      map[string]string{},
		Networks:    []NetworkInfo{{NetworkName: ".imds-0", NetworkID: "net1"}},
	}
	tracker.mu.Unlock()

	settingsMutex.Lock()
	settings.CustomIPs = []string{"169.254.169.254"}
	settings.NetworkConfig = []NetworkConfig{{Name: ".imds-0", IPv4Subnet: "169.254.169.0/24", ProxyIPv4: "169.254.169.254"}}
	settingsMutex.Unlock()
	t.Cleanup(func() {
		settingsMutex.Lock()
		settings.CustomIPs = nil
		settings.NetworkConfig = nil
		settingsMutex.Unlock()
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/containers", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := getContainers(c); err != nil {
		t.Fatalf("getContainers() returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("want status 200, got %d", rec.Code)
	}

	var result ContainersAPIResponse
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(result.Containers) != 1 {
		t.Fatalf("want 1 container, got %d", len(result.Containers))
	}
	if len(result.Containers[0].Addresses) != 1 {
		t.Fatalf("want 1 address, got %d", len(result.Containers[0].Addresses))
	}
	addr := result.Containers[0].Addresses[0]
	if addr.IP != "169.254.169.254" || !addr.Connected {
		t.Errorf("want 169.254.169.254 connected, got %+v", addr)
	}
}

func TestBuildAddressStatusesConnected(t *testing.T) {
	nets := []NetworkInfo{{NetworkName: ".imds-0"}}
	cfg := []NetworkConfig{{Name: ".imds-0", IPv4Subnet: "169.254.169.0/24", ProxyIPv4: "169.254.169.254"}}
	ips := []string{"169.254.169.254"}

	got := buildAddressStatuses(nets, cfg, ips)
	if len(got) != 1 {
		t.Fatalf("want 1 address, got %d", len(got))
	}
	if got[0].IP != "169.254.169.254" || !got[0].Connected {
		t.Errorf("want {169.254.169.254, true}, got %+v", got[0])
	}
}

func TestBuildAddressStatusesNotConnected(t *testing.T) {
	nets := []NetworkInfo{} // container not on any IMDS network
	cfg := []NetworkConfig{{Name: ".imds-0", IPv4Subnet: "169.254.169.0/24", ProxyIPv4: "169.254.169.254"}}
	ips := []string{"169.254.169.254"}

	got := buildAddressStatuses(nets, cfg, ips)
	if len(got) != 1 {
		t.Fatalf("want 1 address, got %d", len(got))
	}
	if got[0].Connected {
		t.Errorf("want not connected, got %+v", got[0])
	}
}

func TestBuildAddressStatusesIPv6(t *testing.T) {
	nets := []NetworkInfo{{NetworkName: ".imds-0"}}
	cfg := []NetworkConfig{{Name: ".imds-0", IPv6Subnet: "fd00:ec2::/64", ProxyIPv6: "fd00:ec2::254"}}
	ips := []string{"fd00:ec2::254"}

	got := buildAddressStatuses(nets, cfg, ips)
	if len(got) != 1 || !got[0].Connected {
		t.Errorf("want IPv6 connected, got %+v", got)
	}
}

func TestBuildAddressStatusesMultiple(t *testing.T) {
	nets := []NetworkInfo{{NetworkName: ".imds-0"}}
	cfg := []NetworkConfig{
		{Name: ".imds-0", IPv4Subnet: "169.254.169.0/24", ProxyIPv4: "169.254.169.254", IPv6Subnet: "fd00:ec2::/64", ProxyIPv6: "fd00:ec2::254"},
	}
	ips := []string{"169.254.169.254", "fd00:ec2::254"}

	got := buildAddressStatuses(nets, cfg, ips)
	if len(got) != 2 {
		t.Fatalf("want 2 addresses, got %d", len(got))
	}
	for _, s := range got {
		if !s.Connected {
			t.Errorf("want %s connected, got not connected", s.IP)
		}
	}
}

func TestBuildAddressStatusesNoConfig(t *testing.T) {
	nets := []NetworkInfo{{NetworkName: ".imds-0"}}
	got := buildAddressStatuses(nets, nil, []string{"169.254.169.254"})
	if len(got) != 1 || got[0].Connected {
		t.Errorf("want not connected when no network config, got %+v", got)
	}
}

func TestIpInNetworkConfigIPv4(t *testing.T) {
	cfg := NetworkConfig{IPv4Subnet: "169.254.169.0/24"}
	if !ipInNetworkConfig("169.254.169.254", cfg) {
		t.Error("want 169.254.169.254 in 169.254.169.0/24")
	}
	if ipInNetworkConfig("10.0.0.1", cfg) {
		t.Error("want 10.0.0.1 not in 169.254.169.0/24")
	}
}

func TestIpInNetworkConfigIPv6(t *testing.T) {
	cfg := NetworkConfig{IPv6Subnet: "fd00:ec2::/64"}
	if !ipInNetworkConfig("fd00:ec2::254", cfg) {
		t.Error("want fd00:ec2::254 in fd00:ec2::/64")
	}
	if ipInNetworkConfig("fd00:a9fe::/64", cfg) {
		t.Error("want fd00:a9fe:: not in fd00:ec2::/64")
	}
}

func TestIpInNetworkConfigInvalidIP(t *testing.T) {
	cfg := NetworkConfig{IPv4Subnet: "169.254.169.0/24"}
	if ipInNetworkConfig("not-an-ip", cfg) {
		t.Error("want invalid IP to return false")
	}
}

func TestGetProxyComposeSuccess(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "compose.yaml")
	content := []byte("version: '3'\nservices:\n  proxy:\n    image: test\n")
	if err := os.WriteFile(yamlPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	old := proxyComposePath
	proxyComposePath = yamlPath
	t.Cleanup(func() { proxyComposePath = old })

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/proxy-compose", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := getProxyCompose(c); err != nil {
		t.Fatalf("getProxyCompose() returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("want status 200, got %d", rec.Code)
	}
	if rec.Body.String() != string(content) {
		t.Errorf("want yaml content %q, got %q", string(content), rec.Body.String())
	}
}

func TestGetProxyComposeMissing(t *testing.T) {
	old := proxyComposePath
	proxyComposePath = "/nonexistent/path/compose.yaml"
	t.Cleanup(func() { proxyComposePath = old })

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/proxy-compose", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := getProxyCompose(c); err != nil {
		t.Fatalf("getProxyCompose() returned error: %v", err)
	}

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want status 500, got %d", rec.Code)
	}
}

func TestGetComposeProjectNameSuccess(t *testing.T) {
	cli := &fakeDockerClient{
		inspectSequence: []container.InspectResponse{
			{
				Config: &container.Config{
					Labels: map[string]string{
						"com.docker.compose.project":            "barnacle-imds-proxy",
						"com.docker.compose.project.config_files": "/path/to/docker-compose.yaml",
					},
				},
			},
		},
	}
	withDockerClient(t, cli)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/compose-project-name", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := getComposeProjectName(c); err != nil {
		t.Fatalf("getComposeProjectName() returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("want status 200, got %d", rec.Code)
	}

	var got map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got["projectName"] != "barnacle-imds-proxy" {
		t.Errorf("want projectName %q, got %q", "barnacle-imds-proxy", got["projectName"])
	}
	if got["configFiles"] != "/path/to/docker-compose.yaml" {
		t.Errorf("want configFiles %q, got %q", "/path/to/docker-compose.yaml", got["configFiles"])
	}
}

func TestGetComposeProjectNameMissingLabel(t *testing.T) {
	cli := &fakeDockerClient{
		inspectSequence: []container.InspectResponse{
			{Config: &container.Config{Labels: map[string]string{}}},
		},
	}
	withDockerClient(t, cli)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/compose-project-name", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := getComposeProjectName(c); err != nil {
		t.Fatalf("getComposeProjectName() returned error: %v", err)
	}

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want status 404, got %d", rec.Code)
	}
}

func TestGetComposeProjectNameInspectError(t *testing.T) {
	cli := &fakeDockerClient{inspectErr: errors.New("container not found")}
	withDockerClient(t, cli)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/compose-project-name", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := getComposeProjectName(c); err != nil {
		t.Fatalf("getComposeProjectName() returned error: %v", err)
	}

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want status 500, got %d", rec.Code)
	}
}

func TestHandleProxyGetSettingsSuccess(t *testing.T) {
	withSettings(t, Settings{URL: "http://proxy.example.com"})

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()

	handleProxyGetSettings(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want status 200, got %d", rec.Code)
	}

	var got Settings
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.URL != "http://proxy.example.com" {
		t.Errorf("want URL %q, got %q", "http://proxy.example.com", got.URL)
	}
}
