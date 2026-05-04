# Barnacle IMDS Proxy - Manual Test Plan

Run on each platform: **macOS**, **Windows**, **Linux**.

---

## Prerequisites

- Docker Desktop installed and running
- Barnacle IMDS Proxy extension installed
- A test IMDS server running and reachable. Start one with:

  ```bash
  docker run --rm -p 8080:8080 -e HTTP_PORT=8080 mendhak/http-https-echo:latest
  ```

  This echoes every request back as JSON including all headers, so you can verify `X-Container-Id`, `X-Container-Name`, and label headers arrive correctly. Keep this terminal visible while testing section 13.

---

## 1. Initial load

| # | Action | Expected |
|---|--------|----------|
| 1.1 | Open the extension | Header with logo and "Barnacle IMDS Proxy" title is visible |
| 1.2 | | "View documentation" link is visible in the top-right |
| 1.3 | | "Containers" tab is active by default |
| 1.4 | | Loading skeleton appears briefly, then resolves |
| 1.5 | Tab from the browser/app focus into the extension | Focus enters the header area |
| 1.6 | Tab through the header | "View documentation" link is reachable and visibly focused |
| 1.7 | Tab to the tab bar | "Containers" and "Settings" tabs are reachable |
| 1.8 | Press Enter or Space on a tab | Tab switches |

---

## 2. Containers tab - empty state

zsh/bash:
```shell
docker rm -f $(docker ps -q --filter label=imds-proxy.enabled=true) 2>/dev/null || true
```

PowerShell:
```powershell
$ids = docker ps -q --filter label=imds-proxy.enabled=true; if ($ids) { docker rm -f $ids }
```

| # | Action | Expected |
|---|--------|----------|
| 2.1 | Ensure no labeled containers are running | "No labeled containers are running." message is shown centered in the table area |
| 2.2 | | Item count at bottom-right reads "Showing 0 items" |
| 2.3 | Tab through the empty state | Focus does not get trapped; label code element and "View documentation" link remain reachable |

---

## 3. Label copy affordance

| # | Action | Expected |
|---|--------|----------|
| 3.1 | Observe the label hint line above the table | `imds-proxy.enabled=true` code element is visible with a small copy icon inside it |
| 3.2 | Click the code element | Snackbar shows "Copied label to clipboard"; clipboard contains `imds-proxy.enabled=true` |
| 3.3 | Tab to the code element, press Enter | Same clipboard and snackbar result as 3.2 |
| 3.4 | Tab to the code element, press Space | Same result |
| 3.5 | Hover over the code element | Background darkens slightly |

---

## 4. Containers tab - with labeled containers

```shell
docker run -d --rm --name test-imds-1 --label imds-proxy.enabled=true alpine sleep 3600
docker run -d --rm --name test-imds-2 --label imds-proxy.enabled=true alpine sleep 3600
```

| # | Action | Expected |
|---|--------|----------|
| 4.1 | Containers appear in the table | Row shows container name, truncated ID, per-IP address chips, and a collapse arrow |
| 4.2 | | Item count at bottom-right updates |
| 4.3 | Hover a row | Name copy icon and ID copy icon appear |
| 4.4 | Click name copy icon | Snackbar "Copied container name to clipboard"; clipboard contains the name |
| 4.5 | Click ID copy icon | Snackbar "Copied container ID to clipboard"; clipboard contains the full ID |
| 4.6 | Click a row | Row expands to show a "Labels" section with key/value pairs in monospace |
| 4.7 | Click the row again | Row collapses |
| 4.8 | Click the expand/collapse arrow button directly | Row toggles without triggering name/ID copy |

### Keyboard accessibility

| # | Action | Expected |
|---|--------|----------|
| 4.9 | Tab to a row | Row receives visible focus |
| 4.10 | Press Enter on focused row | Row expands |
| 4.11 | Press Space on focused row | Row expands/collapses |
| 4.12 | Tab into an expanded row | Focus moves into the label content area |
| 4.13 | Tab to the name copy icon | Icon is focusable; press Enter copies the name |
| 4.14 | Tab to the ID copy icon | Icon is focusable; press Enter copies the full ID |
| 4.15 | Tab to the expand/collapse arrow | Arrow is focusable; press Enter toggles the row |
| 4.16 | Tab past the last interactive element in a row | Focus moves to the next row cleanly |
| 4.17 | Shift+Tab from first element of a row | Focus moves back to the previous row or element |

---

## 5. Network connectivity chips

Requires the containers from section 4 and at least one IP configured in Settings (e.g. `169.254.169.254`).

| # | Action | Expected |
|---|--------|----------|
| 5.1 | Observe the "Networks" column for a running labeled container | One chip per configured IP address |
| 5.2 | | Connected addresses show a green outlined chip |
| 5.3 | | Disconnected addresses show a grey outlined chip |
| 5.4 | Hover a chip | Tooltip reads "Connected" or "Not connected" |
| 5.5 | Stop the proxy container (`docker stop imds-proxy`) | Chips may turn grey (container still tracked but proxy not routing) |
| 5.6 | Start the proxy again | Chips return to green after the next poll |

To confirm network attachment via CLI:

```shell
docker inspect test-imds-1 --format '{{range $k, $v := .NetworkSettings.Networks}}{{$k}} {{end}}'
```

Expected output contains one network per configured IP subnet: `.imds-169.254.169.0` for `169.254.169.254`, `.imds-fd00-ec2--` for `fd00:ec2::254`, etc.

---

## 6. Sorting

Requires at least 2 labeled containers (see section 4).

| # | Action | Expected |
|---|--------|----------|
| 6.1 | Click "Name" column header | Rows sort ascending by name; sort arrow visible |
| 6.2 | Click "Name" again | Rows sort descending |
| 6.3 | Click "Container ID" header | Rows sort ascending by ID |
| 6.4 | Click "Container ID" again | Rows sort descending |
| 6.5 | Tab to "Name" column header | Header receives visible focus |
| 6.6 | Press Enter on focused "Name" header | Rows sort; sort arrow updates |
| 6.7 | Tab to "Container ID" header, press Enter | Sorts by ID |

---

## 7. Proxy container state alerts

### 7a. Stopped

```shell
docker stop imds-proxy
```

| # | Action | Expected |
|---|--------|----------|
| 7.1 | Extension updates | Warning alert: "The IMDS proxy container has stopped - IMDS requests are not being proxied." with a "Start" button |
| 7.2 | Click "Start" | Alert disappears; proxy container returns to running |
| 7.3 | Tab to the "Start" button in the alert | Button receives visible focus |
| 7.4 | Press Enter on focused "Start" button | Same result as click |

### 7b. Paused

```shell
docker pause imds-proxy
```

| # | Action | Expected |
|---|--------|----------|
| 7.5 | Extension updates | Warning alert: "The IMDS proxy container is paused..." with "Unpause" button |
| 7.6 | Tab to "Unpause", press Enter | Alert disappears |

### 7c. Crashed

```shell
docker kill --signal=SIGKILL imds-proxy
```

| # | Action | Expected |
|---|--------|----------|
| 7.7 | Extension updates | Error alert: "The IMDS proxy container has crashed..." with "Start" button |
| 7.8 | Tab to "Start", press Enter | Alert clears |

### 7d. Missing

```shell
docker rm -f imds-proxy
```

| # | Action | Expected |
|---|--------|----------|
| 7.9 | Extension updates | Error alert: "The IMDS proxy container is not running..." with "Start" button |
| 7.10 | Tab to "Start", press Enter | Container is recreated via compose and alert clears |

---

## 8. Backend unreachable

Stop the controller to simulate a dead backend:

```shell
docker stop imds-proxy-controller
```

| # | Action | Expected |
|---|--------|----------|
| 8.1 | Containers tab | Warning alert appears: "Extension backend not responding - list may be outdated" with "Get help" button |
| 8.2 | Settings tab | Warning alert: "Extension backend not responding. Your last saved settings are shown below, but changes cannot be saved." with "Get help" button |
| 8.3 | Settings tab: edit the URL field | Field reverts to the previously saved value after a few seconds |
| 8.4 | Tab to "Get help" button | Button receives visible focus; button is vertically centered in the alert |
| 8.5 | Press Enter on "Get help" | Help dialog opens |
| 8.6 | Dialog recovery steps are listed in order of severity: | |
| | a) Navigate away in Docker Desktop and return to the extension | |
| | b) Disable then re-enable the extension in the Extensions Marketplace | |
| | c) Restart Docker Desktop | |
| | d) Reboot | |
| 8.7 | Tab through the dialog | All links and buttons (including "View the troubleshooting guide" and "Close") are reachable |
| 8.8 | Press Enter on "View the troubleshooting guide" | Opens GitHub troubleshooting URL in browser |
| 8.9 | Press Escape or Tab to "Close" and press Enter | Dialog closes; focus returns to the triggering element |

Restore the controller:

```shell
docker start imds-proxy-controller
```

| # | Action | Expected |
|---|--------|----------|
| 8.10 | Extension recovers | Alert disappears cleanly without flickering back |

---

## 9. Settings tab

| # | Action | Expected |
|---|--------|----------|
| 9.1 | Click "Settings" tab | Settings form visible with "IMDS server URL" field |
| 9.2 | | Previously saved URL is pre-populated |
| 9.3 | | "Save Settings" button is disabled when the field matches the saved value |
| 9.4 | Clear the URL field and click Save | Validation error: "URL is required" |
| 9.5 | Enter `not-a-url` and click Save | Validation error: "Enter a valid URL (e.g. http://localhost:8080)" |
| 9.6 | Enter `http://localhost:8080` and click Save | Button shows "Saving..." briefly, then "Saved"; snackbar "Settings saved" |
| 9.7 | | Button returns to disabled |
| 9.8 | Edit the URL field | Button re-enables and shows "Save Settings" |
| 9.9 | Navigate to Containers tab, return to Settings | Saved URL still shown |

### Keyboard accessibility

| # | Action | Expected |
|---|--------|----------|
| 9.10 | Tab to "Settings" tab, press Enter | Settings tab activates |
| 9.11 | Tab to the URL field | Field receives visible focus |
| 9.12 | Edit the field using keyboard only | Value changes; Save button enables |
| 9.13 | Tab to "Save Settings", press Enter | Save triggers; same result as click |
| 9.14 | Submit an empty field via keyboard | Validation error appears; focus remains near the field |

### External settings update (polling)

zsh/bash:
```shell
docker exec imds-proxy-controller \
  curl -sf --unix-socket /run/guest-services/backend.sock \
  -X POST -H 'Content-Type: application/json' \
  -d '{"url":"http://localhost:9999"}' \
  http://localhost/settings
```

PowerShell:
```powershell
docker exec imds-proxy-controller curl -sf --unix-socket /run/guest-services/backend.sock -X POST -H "Content-Type: application/json" -d '{\"url\":\"http://localhost:9999\"}' http://localhost/settings
```

| # | Action | Expected |
|---|--------|----------|
| 9.15 | Run the command while on the Settings tab | URL field updates to `http://localhost:9999` within ~5 seconds with no skeleton flicker |
| 9.16 | Edit the URL field (leave unsaved), run the external update | External change does NOT overwrite the unsaved edit |

---

## 10. Documentation link

| # | Action | Expected |
|---|--------|----------|
| 10.1 | Click "View documentation" in the header | GitHub repo opens in the system browser (not inside Docker Desktop) |
| 10.2 | Tab to "View documentation", press Enter | Same result as click |
| 10.3 | Tab to "View documentation", press Space | No navigation (correct - Space does not activate links, only Enter does) |

---

## 11. Snackbar behavior

| # | Action | Expected |
|---|--------|----------|
| 11.1 | Save valid settings | Green snackbar appears at bottom-center |
| 11.2 | Wait ~3 seconds | Snackbar auto-dismisses |
| 11.3 | Copy a container name or ID | Green snackbar appears and auto-dismisses |
| 11.4 | Trigger a clipboard error (revoke clipboard permission in OS settings) | Red snackbar appears and does NOT auto-dismiss |
| 11.5 | Click the X on an error snackbar | Dismisses manually |
| 11.6 | Tab to the X button on an error snackbar, press Enter | Dismisses manually |

---

## 12. Light/dark mode

Switch in Docker Desktop → Settings → Appearance.

| # | Action | Expected |
|---|--------|----------|
| 12.1 | Switch to light mode | All text, backgrounds, and alerts render legibly |
| 12.2 | Switch to dark mode | Same check |
| 12.3 | Warning/error alerts readable in both modes | |
| 12.4 | Focus rings visible in both modes | Keyboard focus indicators are clearly visible against the background |

---

## 13. Proxy traffic (functional end-to-end)

Requires the `mendhak/http-https-echo` container running (see Prerequisites) and the extension URL set to `http://localhost:8080`.

```shell
# IPv4 (AWS/GCP)
docker exec test-imds-1 wget -qO- --timeout=5 http://169.254.169.254/status

# IPv6 EC2
docker exec test-imds-1 wget -qO- --timeout=5 http://[fd00:ec2::254]/status

# IPv6 OpenStack
docker exec test-imds-1 wget -qO- --timeout=5 http://[fd00:a9fe:a9fe::254]/status

# Unlabeled container - should fail
docker run --rm alpine wget -qO- --timeout=3 http://169.254.169.254/status
```

| # | Action | Expected |
|---|--------|----------|
| 13.1 | IPv4 request from labeled container | Response from IMDS server |
| 13.2 | IPv6 EC2 request from labeled container | Response from IMDS server |
| 13.3 | IPv6 OpenStack request from labeled container | Response from IMDS server |
| 13.4 | Any request from unlabeled container | Connection refused or no route to host |

---

## Cleanup

```shell
docker rm -f test-imds-1 test-imds-2
```

---

## Platform-specific notes

**macOS:** Verify Docker Desktop VM networking; IPv6 addresses are routed inside the VM.

**Windows:** Docker Desktop requires WSL 2 mode. Verify clipboard copy lands in the Windows clipboard. The `docker exec ... curl` command (section 9) may need to be run from PowerShell rather than WSL to avoid socket path translation issues.

**Linux:** Docker Desktop uses a VM; IMDS networks are created inside it, not on the host. `docker network ls` on the host should still show them via the Docker socket.
