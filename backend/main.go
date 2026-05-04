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
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/sirupsen/logrus"
)

var logger = logrus.New()

const (
	proxySocketPath    = "/var/run/imds-proxy/backend.sock"
	imdsManagedLabel   = "imds-proxy.managed"
	proxyContainerName = "imds-proxy"
)

type ProxyContainerState string

const (
	ProxyStateRunning ProxyContainerState = "running"
	ProxyStatePaused  ProxyContainerState = "paused"
	ProxyStateStopped ProxyContainerState = "stopped"
	ProxyStateFailed  ProxyContainerState = "failed"
	ProxyStateMissing ProxyContainerState = "missing"
)

var proxyNotificationSocketPath = "/var/run/imds-proxy/notifications.sock"

type Settings struct {
	URL              string          `json:"url"`
	CustomIPs        []string        `json:"customIPs,omitempty"`
	NetworkConfig    []NetworkConfig `json:"networkConfig,omitempty"`
}

// NetworkConfig describes a Docker bridge network the backend should manage.
// Computed from CustomIPs; not user-editable.
type NetworkConfig struct {
	Name       string `json:"name"`
	IPv4Subnet string `json:"ipv4Subnet,omitempty"`
	IPv6Subnet string `json:"ipv6Subnet,omitempty"`
	ProxyIPv4  string `json:"proxyIPv4,omitempty"`
	ProxyIPv6  string `json:"proxyIPv6,omitempty"`
}

// subnetToNetworkName converts a subnet CIDR string to a Docker network name.
// e.g. "169.254.169.0/24" → ".imds-169.254.169.0", "fd00:ec2::/64" → ".imds-fd00-ec2--"
func subnetToNetworkName(subnet string) string {
	if i := strings.Index(subnet, "/"); i != -1 {
		subnet = subnet[:i]
	}
	subnet = strings.ReplaceAll(subnet, ":", "-")
	return ".imds-" + subnet
}

// computeNetworkConfig derives the Docker network configuration needed to
// handle the given IMDS addresses. Each subnet gets its own bridge network,
// named after the subnet for stability across add/remove operations.
func computeNetworkConfig(ips []string) []NetworkConfig {
	seen := make(map[string]bool)
	var configs []NetworkConfig

	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}
		parsed := net.ParseIP(ip)
		if parsed == nil {
			continue
		}
		var cfg NetworkConfig
		var subnetStr string
		if parsed.To4() != nil {
			v4 := parsed.To4()
			subnet := net.IPNet{IP: net.IPv4(v4[0], v4[1], v4[2], 0), Mask: net.CIDRMask(24, 32)}
			subnetStr = subnet.String()
			if seen[subnetStr] {
				continue
			}
			cfg = NetworkConfig{
				Name:       subnetToNetworkName(subnetStr),
				IPv4Subnet: subnetStr,
				ProxyIPv4:  ip,
			}
		} else {
			v6 := parsed.To16()
			prefix := make(net.IP, 16)
			copy(prefix, v6[:8])
			subnet := net.IPNet{IP: prefix, Mask: net.CIDRMask(64, 128)}
			subnetStr = subnet.String()
			if seen[subnetStr] {
				continue
			}
			cfg = NetworkConfig{
				Name:       subnetToNetworkName(subnetStr),
				IPv6Subnet: subnetStr,
				ProxyIPv6:  ip,
			}
		}
		seen[subnetStr] = true
		configs = append(configs, cfg)
	}
	return configs
}

type ProxyLookupRequest struct {
	IP string `json:"ip"`
}

type ProxyLookupResponse struct {
	ContainerID string            `json:"containerId"`
	Name        string            `json:"name"`
	Labels      map[string]string `json:"labels"`
}

type NetworkInfo struct {
	NetworkID   string `json:"networkId"`
	NetworkName string `json:"networkName"`
	IPAddress   string `json:"ipAddress,omitempty"`
	IPv6Address string `json:"ipv6Address,omitempty"`
}

// AddressStatus is the per-container API representation of a single configured
// IMDS IP address's connectivity state.
type AddressStatus struct {
	IP        string `json:"ip"`
	Connected bool   `json:"connected"`
}

type ContainerInfo struct {
	ContainerID string            `json:"containerId"`
	Name        string            `json:"name"`
	Labels      map[string]string `json:"labels"`
	Networks    []NetworkInfo     `json:"-"`
	Addresses   []AddressStatus   `json:"addresses"`
}

type ContainersAPIResponse struct {
	Containers  []ContainerInfo     `json:"containers"`
	ProxyStatus ProxyContainerState `json:"proxyStatus"`
}

type containerTracker struct {
	mu              sync.RWMutex
	byID            map[string]ContainerInfo
	ipToContainerID map[string]string
}

var tracker = &containerTracker{
	byID:            make(map[string]ContainerInfo),
	ipToContainerID: make(map[string]string),
}

type settingsStore struct {
	mu sync.RWMutex
	v  Settings
}

func (s *settingsStore) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.v
}

func (s *settingsStore) Set(v Settings) {
	s.mu.Lock()
	s.v = v
	s.mu.Unlock()
}

var (
	settings         = &settingsStore{}
	settingsPath     = "/data/settings.json"
	proxyComposePath = "/imds-proxy-compose.yaml"

	managedNetworks      []string
	managedNetworksMutex sync.RWMutex

	dockerClient DockerClient
	shutdownChan = make(chan struct{})
)

var findContainerByIPFn func(ip string) (*ProxyLookupResponse, error) = findContainerByIP

// notifyProxyConfigUpdateFn is a variable so tests can replace it with a no-op
// to avoid spawning background goroutines that race with test cleanup.
var notifyProxyConfigUpdateFn = notifyProxyConfigUpdate

// reconcileNetworksFn is a variable so tests can replace it with a no-op.
var reconcileNetworksFn = reconcileNetworks

func queryProxyContainerState(ctx context.Context, cli DockerClient) ProxyContainerState {
	inspect, err := cli.ContainerInspect(ctx, proxyContainerName)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return ProxyStateMissing
		}
		logger.Warnf("Failed to query proxy container state: %v", err)
		return ProxyStateMissing
	}
	if inspect.ContainerJSONBase == nil || inspect.State == nil {
		return ProxyStateMissing
	}
	return containerSummaryStateToProxyState(inspect.State.Status)
}

func containerSummaryStateToProxyState(state string) ProxyContainerState {
	switch state {
	case "running":
		return ProxyStateRunning
	case "paused":
		return ProxyStatePaused
	case "dead":
		return ProxyStateFailed
	default:
		return ProxyStateStopped
	}
}

type DockerClient interface {
	ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error)
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
	Events(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error)
	NetworkConnect(ctx context.Context, networkID, containerID string, config *network.EndpointSettings) error
	NetworkCreate(ctx context.Context, name string, options network.CreateOptions) (network.CreateResponse, error)
	NetworkDisconnect(ctx context.Context, networkID, containerID string, force bool) error
	NetworkList(ctx context.Context, options network.ListOptions) ([]network.Summary, error)
	NetworkRemove(ctx context.Context, networkID string) error
	ContainerPause(ctx context.Context, containerID string) error
	ContainerUnpause(ctx context.Context, containerID string) error
	Close() error
}

var newDockerClient = func() (DockerClient, error) {
	return client.NewClientWithOpts(client.WithHost("unix:///var/run/docker.sock"), client.WithAPIVersionNegotiation())
}

func main() {
	var socketPath string
	flag.StringVar(&socketPath, "socket", "/run/guest-services/backend.sock", "Unix domain socket to listen on")
	flag.Parse()

	_ = os.RemoveAll(socketPath)

	logger.SetOutput(os.Stdout)

	// Initialize Docker client once at startup
	cli, err := newDockerClient()
	if err != nil {
		logger.Fatalf("Failed to create Docker client: %v", err)
	}
	dockerClient = cli
	defer dockerClient.Close()

	logMiddleware := middleware.LoggerWithConfig(middleware.LoggerConfig{
		Skipper: func(c echo.Context) bool {
			// Skip logging UI polling requests
			if c.Request().Method == http.MethodGet {
				path := c.Path()
				return path == "/settings" || path == "/containers"
			}
			return false
		},
		Format: `{"time":"${time_rfc3339_nano}","id":"${id}",` +
			`"method":"${method}","uri":"${uri}",` +
			`"status":${status},"error":"${error}"` +
			`}` + "\n",
		CustomTimeFormat: "2006-01-02 15:04:05.00000",
		Output:           logger.Writer(),
	})

	logger.Infof("Starting listening on %s\n", socketPath)
	router := echo.New()
	router.HideBanner = true
	router.Use(logMiddleware)
	startURL := ""

	// Load settings from disk
	if err := loadSettings(); err != nil {
		logger.Warnf("Failed to load settings from disk: %v", err)
	}

	// Reconcile networks before starting the HTTP server so the proxy is
	// attached to all IMDS networks before the backend reports itself as ready.
	{
		netConfig := settings.Get().NetworkConfig
		rctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := reconcileNetworksFn(rctx, dockerClient, netConfig); err != nil {
			logger.Warnf("Startup network reconciliation failed: %v", err)
		}
	}

	// Start monitoring Docker events
	go monitorDockerEvents()

	// Start proxy socket server for container IP lookups
	go startProxySocketServer(proxySocketPath)

	ln, err := listen(socketPath)
	if err != nil {
		logger.Fatal(err)
	}
	router.Listener = ln

	router.GET("/hello", hello)
	router.GET("/settings", getSettings)
	router.POST("/settings", saveSettings)
	router.GET("/proxy-compose", getProxyCompose)
	router.GET("/containers", getContainers)
	router.GET("/compose-project-name", getComposeProjectName)

	logger.Fatal(router.Start(startURL))
}

func startProxySocketServer(socketPath string) {
	_ = os.RemoveAll(socketPath)

	ln, err := listen(socketPath)
	if err != nil {
		logger.Errorf("Failed to listen on proxy socket %s: %v", socketPath, err)
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/container-by-ip", handleContainerLookupByIP)
	mux.HandleFunc("/settings", handleProxyGetSettings)

	server := &http.Server{
		Handler: mux,
	}

	logger.Infof("Proxy socket listening on %s", socketPath)
	if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Errorf("Proxy socket server error: %v", err)
	}
}

func listen(path string) (net.Listener, error) {
	return net.Listen("unix", path)
}

func getLocalIP() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}

	addrs, err := net.LookupIP(hostname)
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if addr.To4() != nil && !addr.IsLoopback() {
			return addr.String(), nil
		}
	}

	return "", errors.New("no IPv4 address found")
}

func updateIPIndex(containerID string) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	ctr := tracker.byID[containerID]
	for _, net := range ctr.Networks {
		if net.IPAddress != "" {
			tracker.ipToContainerID[net.IPAddress] = containerID
		}
		if net.IPv6Address != "" {
			tracker.ipToContainerID[net.IPv6Address] = containerID
		}
	}
}

func removeIPIndexForContainer(containerID string, containerInfo ContainerInfo) {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	for _, net := range containerInfo.Networks {
		if net.IPAddress != "" && tracker.ipToContainerID[net.IPAddress] == containerID {
			delete(tracker.ipToContainerID, net.IPAddress)
		}
		if net.IPv6Address != "" && tracker.ipToContainerID[net.IPv6Address] == containerID {
			delete(tracker.ipToContainerID, net.IPv6Address)
		}
	}
}

func shortID(containerID string) string {
	if len(containerID) > 12 {
		return containerID[:12]
	}
	return containerID
}

func hello(ctx echo.Context) error {
	ip, err := getLocalIP()
	if err != nil {
		ip = "unknown"
	}
	return ctx.JSON(http.StatusOK, HTTPMessageBody{Message: ip})
}

type HTTPMessageBody struct {
	Message string
}

func getSettings(ctx echo.Context) error {
	return ctx.JSON(http.StatusOK, settings.Get())
}

func saveSettings(ctx echo.Context) error {
	var newSettings Settings
	if err := ctx.Bind(&newSettings); err != nil {
		logger.Warnf("Invalid settings payload: %v", err)
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request"})
	}

	if newSettings.URL == "" {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "url is required"})
	}

	newSettings.NetworkConfig = computeNetworkConfig(newSettings.CustomIPs)

	settings.Set(newSettings)

	// Persist settings to disk
	if err := persistSettings(); err != nil {
		logger.Errorf("Failed to persist settings to disk: %v", err)
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to save settings"})
	}

	// Reconcile Docker networks to match the new config.
	// Capture the function variable to avoid races with test cleanup.
	reconcileFn := reconcileNetworksFn
	dc := dockerClient // capture to avoid race with test cleanup
	go func() {
		rctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := reconcileFn(rctx, dc, newSettings.NetworkConfig); err != nil {
			logger.Errorf("Failed to reconcile networks after settings save: %v", err)
		}
	}()

	// Notify proxy of config update
	go notifyProxyConfigUpdateFn()

	logger.Infof("Settings saved: url=%s customIPs=%v networks=%d",
		newSettings.URL, newSettings.CustomIPs, len(newSettings.NetworkConfig))
	return ctx.JSON(http.StatusOK, map[string]string{"message": "Settings saved successfully"})
}

func getProxyCompose(ctx echo.Context) error {
	data, err := os.ReadFile(proxyComposePath)
	if err != nil {
		logger.Errorf("Failed to read proxy compose file: %v", err)
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to read proxy compose file"})
	}

	return ctx.Blob(http.StatusOK, "text/yaml", data)
}

func loadSettings() error {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Infof("Settings file does not exist, starting with defaults")
			s := settings.Get()
			s.CustomIPs = []string{"169.254.169.254"}
			s.NetworkConfig = computeNetworkConfig(s.CustomIPs)
			settings.Set(s)
			return nil
		}
		return err
	}

	var loadedSettings Settings
	if err := json.Unmarshal(data, &loadedSettings); err != nil {
		return err
	}

	settings.Set(loadedSettings)
	logger.Infof("Settings loaded from disk: url=%s", loadedSettings.URL)
	return nil
}

func handleProxyGetSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(settings.Get()); err != nil {
		logger.Errorf("Failed to encode settings response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func handleContainerLookupByIP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var request ProxyLookupRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		logger.Warnf("Invalid proxy lookup payload: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if request.IP == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	response, err := findContainerByIPFn(request.IP)
	if err != nil {
		logger.Errorf("Failed to lookup container for IP %s: %v", request.IP, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if response == nil {
		logger.Infof("No container found for IP %s", request.IP)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	logger.Infof("Container lookup for IP %s: id=%s name=%s", request.IP, response.ContainerID, response.Name)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.Errorf("Failed to encode lookup response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func findContainerByIP(ip string) (*ProxyLookupResponse, error) {
	tracker.mu.RLock()
	defer tracker.mu.RUnlock()

	containerID, ok := tracker.ipToContainerID[ip]
	if !ok {
		return nil, nil
	}
	ctr, ok := tracker.byID[containerID]
	if !ok {
		return nil, nil
	}
	return &ProxyLookupResponse{
		ContainerID: ctr.ContainerID,
		Name:        ctr.Name,
		Labels:      ctr.Labels,
	}, nil
}

func persistSettings() error {
	// Ensure settings directory exists
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(settings.Get(), "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		return err
	}

	logger.Debugf("Settings persisted to %s", settingsPath)
	return nil
}

func monitorDockerEvents() {
	ctx := context.Background()

	logger.Infof("Started monitoring Docker container events")

	// If we have saved network config, reconcile networks to match.
	// Otherwise just discover what's already there (compose defaults).
	savedNetConfig := settings.Get().NetworkConfig

	if len(savedNetConfig) > 0 {
		rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if err := reconcileNetworks(rctx, dockerClient, savedNetConfig); err != nil {
			logger.Errorf("Failed to reconcile networks on startup: %v", err)
		}
		cancel()
	} else {
		// No saved config -- discover existing networks (compose defaults)
		if err := discoverManagedNetworks(ctx, dockerClient); err != nil {
			logger.Errorf("Failed to discover managed networks: %v", err)
		}
	}

	// Scan existing containers
	if err := scanExistingContainers(ctx, dockerClient); err != nil {
		logger.Errorf("Failed to scan existing containers: %v", err)
	}

	eventsChan, errChan := dockerClient.Events(ctx, events.ListOptions{})

	for {
		select {
		case <-shutdownChan:
			logger.Infof("Shutting down Docker event monitoring")
			return
		case event := <-eventsChan:
			if event.Type == events.ContainerEventType {
				switch event.Action {
				case "create":
					logger.Infof("Container created: %s (image: %s)", shortID(event.Actor.ID), event.Actor.Attributes["image"])
					// Check if container has the enabled label
					if event.Actor.Attributes["imds-proxy.enabled"] == "true" {
						if err := addContainerToTrackingWithNetwork(ctx, dockerClient, event.Actor.ID, true); err != nil {
							logger.Errorf("Failed to add container to tracking: %v", err)
						}
					}
				case "destroy":
					logger.Infof("Container destroyed: %s", shortID(event.Actor.ID))
					removeContainerFromTracking(event.Actor.ID)
					// Notify proxy to clear cache for this container
					go notifyProxyContainerDestroyed(event.Actor.ID)
				}
			} else if event.Type == events.NetworkEventType {
				switch event.Action {
				case "connect", "disconnect":
					containerID := event.Actor.Attributes["container"]
					if containerID != "" {
						tracker.mu.RLock()
						_, tracked := tracker.byID[containerID]
						tracker.mu.RUnlock()
						if tracked {
							if err := refreshContainerNetworks(ctx, dockerClient, containerID); err != nil {
								logger.Errorf("Failed to refresh networks for container %s: %v", shortID(containerID), err)
							}
						}
					}
				}
			}
		case err := <-errChan:
			if err != nil && err != io.EOF {
				logger.Errorf("Error monitoring Docker events: %v", err)
			}
			return
		}
	}
}

func scanExistingContainers(ctx context.Context, cli DockerClient) error {
	containers, err := cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return err
	}
	logger.Infof("Found %d existing containers. Now scanning for imds-proxy.enabled=true label", len(containers))

	for _, ctr := range containers {
		if ctr.Labels != nil && ctr.Labels["imds-proxy.enabled"] == "true" {
			if err := addContainerToTrackingWithNetwork(ctx, cli, ctr.ID, false); err != nil {
				logger.Errorf("Failed to add existing container to tracking: %v", err)
			}
		}
	}

	logger.Infof("Scanned existing containers, found %d with imds-proxy.enabled=true", len(tracker.byID))
	return nil
}

func addContainerToTrackingWithNetwork(ctx context.Context, cli DockerClient, containerID string, pauseFirst bool) error {
	inspect, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return err
	}

	// Check which IMDS networks the container is already connected to
	connectedNetworks := make(map[string]bool)
	for networkName := range inspect.NetworkSettings.Networks {
		connectedNetworks[networkName] = true
	}

	// Determine which networks need to be connected
	managedNetworksMutex.RLock()
	knownNetworks := managedNetworks
	managedNetworksMutex.RUnlock()

	networksToConnect := []string{}
	for _, networkName := range knownNetworks {
		if !connectedNetworks[networkName] {
			networksToConnect = append(networksToConnect, networkName)
		}
	}

	// Connect to IMDS networks if needed
	if len(networksToConnect) > 0 {
		paused := false
		if pauseFirst && inspect.State.Running {
			logger.Infof("Pausing container %s before connecting to networks", shortID(containerID))
			if err := cli.ContainerPause(ctx, containerID); err != nil {
				logger.Errorf("Failed to pause container %s: %v", shortID(containerID), err)
				// Continue anyway
			} else {
				paused = true
			}
			defer func() {
				if paused {
					logger.Infof("Unpausing container %s after connecting to networks", shortID(containerID))
					if err := cli.ContainerUnpause(ctx, containerID); err != nil {
						logger.Errorf("Failed to unpause container %s: %v", shortID(containerID), err)
					}
				}
			}()
		}

		for _, networkName := range networksToConnect {
			logger.Infof("Connecting container %s to network %s", shortID(containerID), networkName)
			if err := cli.NetworkConnect(ctx, networkName, containerID, &network.EndpointSettings{}); err != nil {
				logger.Errorf("Failed to connect container %s to network %s: %v", shortID(containerID), networkName, err)
				// Continue tracking even if network connection fails
			}
		}
	}

	// Re-inspect to get updated network information
	inspect, err = cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return err
	}

	networks := make([]NetworkInfo, 0)
	for networkName, networkSettings := range inspect.NetworkSettings.Networks {
		networks = append(networks, NetworkInfo{
			NetworkID:   networkSettings.NetworkID,
			NetworkName: networkName,
			IPAddress:   networkSettings.IPAddress,
			IPv6Address: networkSettings.GlobalIPv6Address,
		})
	}

	containerInfo := ContainerInfo{
		ContainerID: inspect.ID,
		Name:        inspect.Name,
		Labels:      inspect.Config.Labels,
		Networks:    networks,
	}

	if containerInfo.Labels == nil {
		containerInfo.Labels = make(map[string]string)
	}

	tracker.mu.Lock()
	tracker.byID[containerID] = containerInfo
	tracker.mu.Unlock()

	updateIPIndex(containerID)

	logger.Infof("Added container to tracking: %s (%s)", shortID(containerID), inspect.Name)
	return nil
}

func removeContainerFromTracking(containerID string) {
	tracker.mu.Lock()
	info, exists := tracker.byID[containerID]
	if exists {
		delete(tracker.byID, containerID)
	}
	tracker.mu.Unlock()

	if exists {
		removeIPIndexForContainer(containerID, info)
		logger.Infof("Removed container from tracking: %s (%s)", shortID(containerID), info.Name)
	}
}

func refreshContainerNetworks(ctx context.Context, cli DockerClient, containerID string) error {
	inspect, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return err
	}

	networks := make([]NetworkInfo, 0)
	for networkName, networkSettings := range inspect.NetworkSettings.Networks {
		networks = append(networks, NetworkInfo{
			NetworkID:   networkSettings.NetworkID,
			NetworkName: networkName,
			IPAddress:   networkSettings.IPAddress,
			IPv6Address: networkSettings.GlobalIPv6Address,
		})
	}

	tracker.mu.Lock()
	info, exists := tracker.byID[containerID]
	if exists {
		info.Networks = networks
		tracker.byID[containerID] = info
	}
	tracker.mu.Unlock()

	if exists {
		updateIPIndex(containerID)
		logger.Infof("Refreshed networks for container %s", shortID(containerID))
	}

	return nil
}

// ipInNetworkConfig reports whether ip falls within the subnet covered by cfg.
func ipInNetworkConfig(ip string, cfg NetworkConfig) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	if parsed.To4() != nil {
		if cfg.IPv4Subnet == "" {
			return false
		}
		_, subnet, err := net.ParseCIDR(cfg.IPv4Subnet)
		if err != nil {
			return false
		}
		return subnet.Contains(parsed)
	}
	if cfg.IPv6Subnet == "" {
		return false
	}
	_, subnet, err := net.ParseCIDR(cfg.IPv6Subnet)
	if err != nil {
		return false
	}
	return subnet.Contains(parsed)
}

// buildAddressStatuses returns one AddressStatus per configured IP, reporting
// whether the container is connected to the network that covers that address.
func buildAddressStatuses(containerNetworks []NetworkInfo, netConfig []NetworkConfig, ips []string) []AddressStatus {
	connectedNames := make(map[string]bool, len(containerNetworks))
	for _, n := range containerNetworks {
		connectedNames[n.NetworkName] = true
	}
	statuses := make([]AddressStatus, 0, len(ips))
	for _, ip := range ips {
		connected := false
		for _, cfg := range netConfig {
			if ipInNetworkConfig(ip, cfg) {
				connected = connectedNames[cfg.Name]
				break
			}
		}
		statuses = append(statuses, AddressStatus{IP: ip, Connected: connected})
	}
	return statuses
}

func discoverManagedNetworks(ctx context.Context, cli DockerClient) error {
	networkList, err := cli.NetworkList(ctx, network.ListOptions{
		Filters: filters.NewArgs(filters.Arg("label", imdsManagedLabel+"=true")),
	})
	if err != nil {
		return err
	}

	discovered := make([]string, 0, len(networkList))
	for _, n := range networkList {
		discovered = append(discovered, n.Name)
	}

	managedNetworksMutex.Lock()
	managedNetworks = discovered
	managedNetworksMutex.Unlock()

	logger.Infof("Discovered %d managed IMDS network(s)", len(discovered))
	return nil
}

// reconcileNetworks creates, removes, and reconnects Docker networks so the
// actual state matches the desired NetworkConfig. It also reconnects the proxy
// container and all tracked containers to the new networks.
func reconcileNetworks(ctx context.Context, cli DockerClient, desired []NetworkConfig) error {
	// 1. List existing managed networks
	existing, err := cli.NetworkList(ctx, network.ListOptions{
		Filters: filters.NewArgs(filters.Arg("label", imdsManagedLabel+"=true")),
	})
	if err != nil {
		return fmt.Errorf("listing managed networks: %w", err)
	}

	existingByName := make(map[string]network.Summary, len(existing))
	for _, n := range existing {
		existingByName[n.Name] = n
	}

	desiredByName := make(map[string]NetworkConfig, len(desired))
	for _, d := range desired {
		desiredByName[d.Name] = d
	}

	// 2. Remove networks that are no longer needed or whose subnets changed.
	// Docker bridge networks can't be modified in place, so a subnet change
	// requires removing and recreating the network.
	for name, n := range existingByName {
		d, want := desiredByName[name]
		keep := want && networkSubnetsMatch(n, d)
		if keep {
			continue
		}
		reason := "unneeded"
		if want {
			reason = "subnet changed"
		}
		logger.Infof("Removing network %s (%s)", name, reason)
		if err := disconnectAllFromNetwork(ctx, cli, name); err != nil {
			logger.Warnf("Failed to disconnect containers from %s: %v", name, err)
		}
		if err := cli.NetworkRemove(ctx, n.ID); err != nil {
			logger.Errorf("Failed to remove network %s: %v", name, err)
			continue
		}
		delete(existingByName, name)
	}

	// 3. Create missing networks
	for _, d := range desired {
		if _, exists := existingByName[d.Name]; exists {
			continue
		}
		logger.Infof("Creating network %s (v4=%s v6=%s)", d.Name, d.IPv4Subnet, d.IPv6Subnet)
		ipamConfigs := []network.IPAMConfig{}
		if d.IPv4Subnet != "" {
			ipamConfigs = append(ipamConfigs, network.IPAMConfig{Subnet: d.IPv4Subnet})
		}
		if d.IPv6Subnet != "" {
			ipamConfigs = append(ipamConfigs, network.IPAMConfig{Subnet: d.IPv6Subnet})
		}

		enableIPv6 := d.IPv6Subnet != ""
		opts := network.CreateOptions{
			Driver:     "bridge",
			EnableIPv6: &enableIPv6,
			IPAM: &network.IPAM{
				Config: ipamConfigs,
			},
			Labels: map[string]string{
				imdsManagedLabel: "true",
			},
		}
		if _, err := cli.NetworkCreate(ctx, d.Name, opts); err != nil {
			logger.Errorf("Failed to create network %s: %v", d.Name, err)
			continue
		}
	}

	// 4. Connect proxy container to new networks with correct IPs
	for _, d := range desired {
		endpointConfig := &network.EndpointSettings{}
		if d.ProxyIPv4 != "" || d.ProxyIPv6 != "" {
			ipamConfig := &network.EndpointIPAMConfig{}
			if d.ProxyIPv4 != "" {
				ipamConfig.IPv4Address = d.ProxyIPv4
			}
			if d.ProxyIPv6 != "" {
				ipamConfig.IPv6Address = d.ProxyIPv6
			}
			endpointConfig.IPAMConfig = ipamConfig
		}
		if err := cli.NetworkConnect(ctx, d.Name, proxyContainerName, endpointConfig); err != nil {
			// May already be connected -- log but don't fail
			logger.Debugf("Connect proxy to %s: %v", d.Name, err)
		}
	}

	// 5. Reconnect tracked containers to the new networks
	tracker.mu.RLock()
	containerIDs := make([]string, 0, len(tracker.byID))
	for id := range tracker.byID {
		containerIDs = append(containerIDs, id)
	}
	tracker.mu.RUnlock()

	for _, cid := range containerIDs {
		for _, d := range desired {
			if err := cli.NetworkConnect(ctx, d.Name, cid, nil); err != nil {
				logger.Debugf("Connect container %s to %s: %v", shortID(cid), d.Name, err)
			}
		}
	}

	// 6. Refresh the in-memory managed networks cache
	if err := discoverManagedNetworks(ctx, cli); err != nil {
		logger.Warnf("Failed to refresh managed networks after reconciliation: %v", err)
	}

	logger.Infof("Network reconciliation complete: %d network(s) configured", len(desired))
	return nil
}

// networkSubnetsMatch reports whether an existing managed network's IPAM
// subnets exactly match the desired NetworkConfig (set-equal on subnet CIDR).
func networkSubnetsMatch(existing network.Summary, desired NetworkConfig) bool {
	want := map[string]bool{}
	if desired.IPv4Subnet != "" {
		want[desired.IPv4Subnet] = true
	}
	if desired.IPv6Subnet != "" {
		want[desired.IPv6Subnet] = true
	}
	have := map[string]bool{}
	for _, c := range existing.IPAM.Config {
		if c.Subnet != "" {
			have[c.Subnet] = true
		}
	}
	if len(want) != len(have) {
		return false
	}
	for s := range want {
		if !have[s] {
			return false
		}
	}
	return true
}

// disconnectAllFromNetwork disconnects all containers from a network.
// networkName is the user-facing network name (matches keys in
// c.NetworkSettings.Networks); NetworkDisconnect accepts either name or ID.
func disconnectAllFromNetwork(ctx context.Context, cli DockerClient, networkName string) error {
	containers, err := cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return err
	}
	for _, c := range containers {
		if _, attached := c.NetworkSettings.Networks[networkName]; attached {
			_ = cli.NetworkDisconnect(ctx, networkName, c.ID, true)
		}
	}
	return nil
}

func getContainers(ctx echo.Context) error {
	tracker.mu.RLock()
	defer tracker.mu.RUnlock()

	s := settings.Get()
	netConfig := s.NetworkConfig
	customIPs := s.CustomIPs

	containerList := make([]ContainerInfo, 0, len(tracker.byID))
	for _, info := range tracker.byID {
		info.Addresses = buildAddressStatuses(info.Networks, netConfig, customIPs)
		containerList = append(containerList, info)
	}

	queryCtx, cancel := context.WithTimeout(ctx.Request().Context(), 3*time.Second)
	defer cancel()
	proxyStatus := queryProxyContainerState(queryCtx, dockerClient)

	return ctx.JSON(http.StatusOK, ContainersAPIResponse{
		Containers:  containerList,
		ProxyStatus: proxyStatus,
	})
}

func getComposeProjectName(ctx echo.Context) error {
	hostname, err := os.Hostname()
	if err != nil {
		logger.Errorf("Failed to get hostname: %v", err)
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to get hostname"})
	}

	queryCtx, cancel := context.WithTimeout(ctx.Request().Context(), 3*time.Second)
	defer cancel()

	inspect, err := dockerClient.ContainerInspect(queryCtx, hostname)
	if err != nil {
		logger.Errorf("Failed to inspect container %s: %v", hostname, err)
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to inspect container"})
	}

	projectName := ""
	configFiles := ""
	if inspect.Config != nil && inspect.Config.Labels != nil {
		projectName = inspect.Config.Labels["com.docker.compose.project"]
		configFiles = inspect.Config.Labels["com.docker.compose.project.config_files"]
	}

	if projectName == "" {
		logger.Warnf("No com.docker.compose.project label found on container %s", hostname)
		return ctx.JSON(http.StatusNotFound, map[string]string{"error": "Compose project name not found"})
	}

	return ctx.JSON(http.StatusOK, map[string]string{"projectName": projectName, "configFiles": configFiles})
}

func notifyProxyConfigUpdate() {
	maxRetries := 3
	backoff := 100 * time.Millisecond
	socketPath := proxyNotificationSocketPath

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(backoff)
			backoff *= 2
		}

		client := &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", socketPath)
				},
			},
			Timeout: 5 * time.Second,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/config-updated", nil)
		if err != nil {
			logger.Errorf("Failed to create proxy notification request: %v", err)
			return
		}

		resp, err := client.Do(req)
		if err != nil {
			logger.Warnf("Attempt %d/%d: Failed to notify proxy of config update: %v", attempt+1, maxRetries, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			logger.Warnf("Attempt %d/%d: Proxy notification returned status %d", attempt+1, maxRetries, resp.StatusCode)
			continue
		}

		logger.Infof("Proxy notified of configuration update")
		return
	}

	logger.Errorf("Failed to notify proxy of config update after %d attempts", maxRetries)
}

func notifyProxyContainerDestroyed(containerID string) {
	maxRetries := 3
	backoff := 100 * time.Millisecond
	socketPath := proxyNotificationSocketPath

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(backoff)
			backoff *= 2
		}

		client := &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", socketPath)
				},
			},
			Timeout: 5 * time.Second,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		payload := map[string]string{"containerId": containerID}
		body, err := json.Marshal(payload)
		if err != nil {
			cancel()
			logger.Errorf("Failed to marshal container destroyed payload: %v", err)
			return
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix/container-destroyed", bytes.NewReader(body))
		if err != nil {
			cancel()
			logger.Errorf("Failed to create proxy container destroyed notification request: %v", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		cancel()

		if err != nil {
			logger.Warnf("Attempt %d/%d: Failed to notify proxy of container destruction: %v", attempt+1, maxRetries, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			logger.Warnf("Attempt %d/%d: Proxy container destroyed notification returned status %d", attempt+1, maxRetries, resp.StatusCode)
			continue
		}

		logger.Infof("Proxy notified of container %s destruction", shortID(containerID))
		return
	}

	logger.Errorf("Failed to notify proxy of container %s destruction after %d attempts", shortID(containerID), maxRetries)
}
