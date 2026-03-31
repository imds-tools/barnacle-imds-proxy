# Architecture

This document describes how Barnacle IMDS Proxy works internally.

## Overview

The extension runs two containers inside the Docker Desktop VM: a **controller** and a **proxy**. Neither runs on your host machine. They communicate via a Unix socket exposed through Docker Desktop's guest services API.

These containers are hidden from the Docker Desktop UI and from `docker ps` by default. To see them, enable **Show Docker Extensions system containers** in Docker Desktop: Settings > Extensions.

```
Host machine
  └── Docker Desktop VM
        ├── imds-proxy-controller   (watches Docker events, manages networks)
        ├── imds-proxy              (binds IMDS addresses, forwards requests)
        └── your labeled containers (reach proxy via IMDS bridge networks)
```

Your IMDS server runs wherever you want, typically on the host at `localhost`.

## Controller

The controller uses the Docker socket to watch for container lifecycle events. When a container starts with the `imds-proxy.enabled=true` label, the controller:

1. Pauses the container
2. Connects it to the IMDS bridge networks
3. Unpauses it

Pausing before connecting ensures the IMDS addresses are routable by the time the container's process starts. Without this, a process that tries to hit the IMDS endpoint at startup could get a connection refused before the network is ready.

The controller also handles container stop/destroy events to clean up its internal tracking.

## Proxy

The proxy binds to the IMDS link-local addresses inside the VM and listens for HTTP traffic on each. When a request comes in, it:

1. Looks up which container the request came from by source IP
2. Forwards the request to the configured IMDS server URL
3. Adds identity headers to the forwarded request:
   - `X-Container-Id` - full container ID
   - `X-Container-Name` - container name
   - One header per container label (`X-Label-<key>: <value>`)
4. Streams the response back to the container

If the configured URL uses `localhost`, the proxy rewrites it to `host.docker.internal` before forwarding. This is necessary because inside the VM, `localhost` refers to the VM itself, not the host machine.

## Networks

Two bridge networks are created inside the VM:

- **`.imds-0`** - carries IPv4 (`169.254.169.254`) and EC2 IPv6 (`fd00:ec2::254`) traffic
- **`.imds-1`** - carries OpenStack IPv6 (`fd00:a9fe:a9fe::254`) traffic

Both are attached to a container when it starts. The provider connectivity status shown in the UI reflects whether each network is connected.

| Provider  | Network  | IPv4             | IPv6                  |
|-----------|----------|------------------|-----------------------|
| AWS       | `.imds-0` | 169.254.169.254  | fd00:ec2::254         |
| GCP       | `.imds-0` | 169.254.169.254  | fd00:ec2::254         |
| OpenStack | `.imds-1` |                  | fd00:a9fe:a9fe::254   |

## Settings

Settings (the IMDS server URL) are stored in the backend and served via a Unix socket at `/run/guest-services/backend.sock`. The UI reads settings on load and polls for changes every 5 seconds. If the backend is unreachable, the UI falls back to `localStorage` for read-only display.

## UI

The extension UI is a React app using the Docker Desktop MUI theme. It communicates with the backend over the Docker Desktop extension API (HTTP over the guest services socket). The Containers tab polls the backend every few seconds to reflect the live state of running labeled containers.
