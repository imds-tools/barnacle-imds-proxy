# Troubleshooting

## Extension backend not responding

The extension UI shows a warning banner if it can't reach the controller. This usually means the `imds-proxy-controller` container stopped or crashed.

Try these steps in order:

1. **Navigate away and back.** Click on another Docker Desktop section (Containers, Images, etc.) and then return to the extension. Docker Desktop sometimes loses the connection to an extension's backend after the VM is idle.

2. **Disable and re-enable the extension.** Open Docker Desktop settings, go to Extensions, find Barnacle IMDS Proxy, disable it, and re-enable it. This restarts the extension's containers.

3. **Restart Docker Desktop.**

4. **Reboot.** If nothing else works, a reboot clears any VM networking issues.

To check whether the containers are actually running:

```bash
docker ps --filter name=imds-proxy
```

You should see `imds-proxy-controller` and `imds-proxy` both in `Up` status.

> **Note:** Extension containers are hidden by default. If the command returns nothing, enable **Show Docker Extensions system containers** in Docker Desktop: Settings > Extensions. Without that, `docker ps` and `docker ps -a` won't show them.

To check logs for errors:

```bash
docker logs imds-proxy-controller
docker logs imds-proxy
```

---

## IMDS requests not reaching my server

**Check the URL is saved.** Open the Settings tab and confirm the URL field shows what you expect. If it's empty, the proxy has nowhere to forward requests.

**Check the proxy is running.** The Containers tab shows a warning if the proxy container has stopped or crashed.

**Check the container has the label.** Only containers with `imds-proxy.enabled=true` are attached to the IMDS networks. Containers without the label get connection refused.

```bash
docker inspect <container-name> --format '{{index .Config.Labels "imds-proxy.enabled"}}'
```

Should output `true`.

**Check network attachment.** The Networks column in the Containers tab shows a chip per configured IP — green means connected. To check directly:

```bash
docker inspect <container-name> --format '{{range $k, $v := .NetworkSettings.Networks}}{{$k}} {{end}}'
```

Look for names starting with `.imds-` (e.g. `.imds-169.254.169.0`).

**Check the IMDS address is reachable from the container.**

```bash
docker exec <container-name> wget -qO- --timeout=5 http://169.254.169.254/
```

If that times out but the container is attached to the IMDS network, the proxy may not be running.

**Check your server is listening on `0.0.0.0`.** The proxy forwards requests from inside the Docker Desktop VM using `host.docker.internal`. If your server is bound to `127.0.0.1` only, those connections will be refused. Make sure it listens on `0.0.0.0`.

---

## Network chips are grey

Grey means the container is not connected to that IMDS network. This usually happens right after a container starts (the controller hasn't finished attaching it yet) or if a network was manually disconnected.

If the chips stay grey after a minute, check whether the IMDS networks exist:

```bash
docker network ls | grep imds
```

You should see one network per configured IP address (e.g. `.imds-169.254.169.0`). If they're missing, reinstalling the extension will recreate them.

---

## Container is not in the Containers tab

The Containers tab only shows containers that have the `imds-proxy.enabled=true` label and are currently running. Stopped containers are not listed.

If a container is running and labeled but not showing up, check whether the backend is reachable (the warning banner would appear if not). You can also check controller logs to see if the attach event was processed:

```bash
docker logs imds-proxy-controller | grep <container-name>
```

---

## Still stuck?

[Open a GitHub issue](https://github.com/imdsutil/barnacle-imds-proxy/issues) with the output of `docker logs imds-proxy-controller` and `docker logs imds-proxy`.
