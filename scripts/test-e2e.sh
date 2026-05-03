#!/usr/bin/env bats
# Copyright 2026 Matt Miller
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# End-to-end tests for barnacle-imds-proxy.
#
# BACKEND controls which upstream server is used:
#   BACKEND=test-server  (default) — starts the built-in Go test server on TEST_SERVER_PORT
#   BACKEND=imds-server             — expects imds-server already running; skips echo-header tests
#
# The extension must be configured to point at the appropriate backend URL before running.

CONTAINER_NAME="imds-e2e-test"
BACKEND="${BACKEND:-test-server}"
TEST_SERVER_PORT="${TEST_SERVER_PORT:-8080}"
IMDS_SERVER_PORT="${IMDS_SERVER_PORT:-3333}"
TEST_SERVER_PID=""
CONTROLLER_SOCKET="/run/guest-services/backend.sock"

# Update the extension settings via the controller's settings API
set_settings() {
    local url="$1"
    docker exec imds-proxy-controller \
        curl -sf --unix-socket "$CONTROLLER_SOCKET" \
        -X POST -H 'Content-Type: application/json' \
        -d "{\"url\":\"$url\",\"customIPs\":[\"169.254.169.254\",\"fd00:ec2::254\"]}" \
        http://localhost/settings >/dev/null
}

setup_file() {
    if [ "$BACKEND" = "test-server" ]; then
        # Build and start the test server
        cd "$BATS_TEST_DIRNAME/../test-server"
        go build -o "$BATS_FILE_TMPDIR/test-server" .
        "$BATS_FILE_TMPDIR/test-server" -port="$TEST_SERVER_PORT" &
        TEST_SERVER_PID=$!
        echo "$TEST_SERVER_PID" > "$BATS_FILE_TMPDIR/test-server.pid"

        # Wait for it to be ready
        for i in $(seq 1 10); do
            if curl -sf "http://localhost:$TEST_SERVER_PORT/status" >/dev/null 2>&1; then
                break
            fi
            sleep 1
        done
        if ! curl -sf "http://localhost:$TEST_SERVER_PORT/status" >/dev/null 2>&1; then
            echo "Test server failed to start on port $TEST_SERVER_PORT" >&2
            return 1
        fi

        set_settings "http://localhost:$TEST_SERVER_PORT"
    else
        set_settings "http://localhost:$IMDS_SERVER_PORT"
    fi

    # Start a labeled container
    docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
    docker run -d --rm --name "$CONTAINER_NAME" --label imds-proxy.enabled=true alpine sleep 3600 >/dev/null

    # Wait for controller to attach networks
    for i in $(seq 1 15); do
        networks=$(docker inspect "$CONTAINER_NAME" --format '{{range $k, $v := .NetworkSettings.Networks}}{{$k}} {{end}}' 2>/dev/null || echo "")
        if echo "$networks" | grep -q '\.imds-0'; then
            return 0
        fi
        sleep 1
    done
    echo "Container was not attached to IMDS networks within 15s" >&2
    return 1
}

teardown_file() {
    docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
    if [ -f "$BATS_FILE_TMPDIR/test-server.pid" ]; then
        kill "$(cat "$BATS_FILE_TMPDIR/test-server.pid")" 2>/dev/null || true
    fi
}

# --- network tests ---

@test ".imds-0 network exists" {
    docker network ls --format '{{.Name}}' | grep -q '\.imds-0'
}

@test "container is attached to .imds-0" {
    networks=$(docker inspect "$CONTAINER_NAME" --format '{{range $k, $v := .NetworkSettings.Networks}}{{$k}} {{end}}')
    echo "$networks" | grep -q '\.imds-0'
}

# --- reachability tests (use /status — present on both backends) ---

@test "IPv4 169.254.169.254 is reachable" {
    docker exec "$CONTAINER_NAME" wget -qO- --timeout=5 http://169.254.169.254/status
}

@test "IPv6 fd00:ec2::254 is reachable" {
    docker exec "$CONTAINER_NAME" wget -qO- --timeout=5 "http://[fd00:ec2::254]/status"
}


# --- proxy header tests (test-server only) ---

@test "X-Container-Id header is forwarded" {
    [ "$BACKEND" = "test-server" ] || skip "requires test-server backend"
    run docker exec "$CONTAINER_NAME" wget -qS --timeout=5 http://169.254.169.254/ -O /dev/null 2>&1
    echo "$output" | grep -qi 'X-Echo-X-Container-Id'
}

@test "X-Container-Name header is forwarded" {
    [ "$BACKEND" = "test-server" ] || skip "requires test-server backend"
    run docker exec "$CONTAINER_NAME" wget -qS --timeout=5 http://169.254.169.254/ -O /dev/null 2>&1
    echo "$output" | grep -qi 'X-Echo-X-Container-Name'
}
