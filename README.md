<img src="logo.svg" width="128" alt="Barnacle IMDS Proxy">

# Barnacle IMDS Proxy

Your local containers can't reach cloud providers' Instance Metadata Service. This extension routes their IMDS requests to an IMDS request handler you run on your machine - no code changes, no static keys baked into your environment.

## What problem does this solve?

Cloud SDKs (AWS, GCP, Azure, etc.) check the Instance Metadata Service typically at `169.254.169.254` for credentials and other config when running in the cloud. Local containers can't reach that address, so you're stuck with one of these workarounds:

| Approach | App code changes? | Secrets in Dockerfile/Compose? | Per-container identity? |
|---|---|---|---|
| Static env vars (`AWS_ACCESS_KEY_ID`, etc.) | None | Yes - env vars per container | Yes - but a static key per container |
| Credential files mounted (`~/.aws`) | None | Yes - volume mount per container | Yes - via `AWS_PROFILE` (or similar), but static keys only (breaks SSO, `credentials_process`, etc.) |
| aws-vault | None | No | No - manual exec wrapper, doesn't work well with Compose |
| LocalStack[^1] | No (usually) | No | No |
| **Barnacle + credential server** | **None** | **No** | **Yes - via container labels** |

[^1]: LocalStack solves a different problem - it mocks AWS services locally so you can test without hitting real APIs. It doesn't provide real credentials and requires your code to target a different endpoint. The others are all credential solutions, just with different tradeoffs.

Barnacle handles the routing. For the credential server, you can use a purpose-built IMDS server like [imds-server](https://github.com/imdsutil/imds-server), or copy a minimal script from [docs/recipes.md](docs/recipes.md) if you just need something quick.

## Install

Search for "Barnacle" in the Docker Desktop Extensions Marketplace.

## Quick start

1. Start a credential server. See [docs/recipes.md](docs/recipes.md) for copy-paste scripts for different cloud provider recipes.

2. Open the extension and go to the **Settings** tab. Enter your server URL. You can use `localhost` - the proxy rewrites it to `host.docker.internal` for you.

3. Add the label `imds-proxy.enabled=true` to any container:

   ```yaml
   services:
     my-app:
       image: my-app:latest
       labels:
         - "imds-proxy.enabled=true"
   ```

   Or with `docker run`:

   ```bash
   docker run --label imds-proxy.enabled=true my-app:latest
   ```

4. Done. The extension connects labeled containers to the IMDS proxy automatically. The Containers tab shows which containers are active and their network connectivity status.

## Supported addresses

| Provider  | Address               | Protocol |
|-----------|-----------------------|----------|
| AWS / GCP | `169.254.169.254`     | IPv4     |
| AWS       | `fd00:ec2::254`       | IPv6     |
| OpenStack | `fd00:a9fe:a9fe::254` | IPv6     |

## How it works

Two services run inside the Docker Desktop VM:

- The **controller** watches Docker events. When a labeled container starts, it briefly pauses it, connects it to the IMDS networks, then unpauses it - ensuring the network is ready before the container's process starts.
- The **proxy** binds to the IMDS addresses and forwards requests to your server, adding `X-Container-Id`, `X-Container-Name`, and container label headers so your server knows which container made the request.

For the full technical description, see [docs/architecture.md](docs/architecture.md).

## Troubleshooting

See [docs/troubleshooting.md](docs/troubleshooting.md).

## Development

See [DEVELOPMENT.md](DEVELOPMENT.md).

## Accessibility

The extension UI targets [WCAG 2.1 Level AA](https://www.w3.org/TR/WCAG21/) conformance. To report an accessibility issue, [open a GitHub issue](https://github.com/imdsutil/barnacle-imds-proxy/issues).

## License

Apache 2.0 - see [LICENSE](LICENSE).
