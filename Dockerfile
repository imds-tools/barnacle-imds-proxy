FROM golang:1.26-alpine@sha256:d4c4845f5d60c6a974c6000ce58ae079328d03ab7f721a0734277e69905473e5 AS builder
ENV CGO_ENABLED=0
WORKDIR /backend
COPY backend/go.* .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download
COPY backend/. .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o bin/service

FROM --platform=$BUILDPLATFORM node:24-alpine AS client-builder
WORKDIR /ui
# cache packages in layer
COPY ui/package.json /ui/package.json
COPY ui/pnpm-lock.yaml /ui/pnpm-lock.yaml
RUN --mount=type=cache,target=/root/.pnpm-store \
    corepack enable && \
    corepack prepare pnpm --activate && \
    pnpm install --frozen-lockfile
# install
COPY ui /ui
COPY description.json /ui/public/description.json
COPY logo.svg /ui/src/logo.svg
RUN pnpm run build

FROM alpine
RUN apk add --no-cache curl
ARG DESCRIPTION="Description not set"
ARG PROXY_IMAGE=barnacle-imds-proxy-proxy:latest
LABEL org.opencontainers.image.title="Barnacle IMDS Proxy" \
    org.opencontainers.image.description="${DESCRIPTION}" \
    org.opencontainers.image.vendor="Matt Miller" \
    com.docker.desktop.extension.api.version="0.4.2" \
    com.docker.extension.screenshots='[{"alt":"Barnacle IMDS Proxy UI","url":"https://raw.githubusercontent.com/imdsutil/barnacle-imds-proxy/refs/heads/main/screenshots/screenshot1.png"}]' \
    com.docker.desktop.extension.icon="https://raw.githubusercontent.com/imdsutil/barnacle-imds-proxy/refs/heads/main/logo.svg" \
    com.docker.extension.detailed-description="<p>Cloud SDKs check 169.254.169.254 for credentials when running on a cloud VM. Local containers can't reach that address. Barnacle routes those requests to a server you control - no code changes, no static keys.</p><p>Pair it with a small credential server to give each container real short-lived credentials based on its labels. Copy-paste scripts for AWS, GCP, and Azure are in the docs. The proxy adds <code>X-Container-Id</code>, <code>X-Container-Name</code>, and label headers so your server knows exactly which container is asking.</p><h3>Setup</h3><ol><li>Start a credential server (see docs/recipes.md for scripts)</li><li>Add the label <code>imds-proxy.enabled=true</code> to any container</li><li>Point the extension at your server URL</li></ol><h3>Supported addresses</h3><ul><li>AWS, GCP: 169.254.169.254 (IPv4)</li><li>AWS: fd00:ec2::254 (IPv6)</li><li>OpenStack: fd00:a9fe:a9fe::254 (IPv6)</li></ul>" \
    com.docker.extension.publisher-url="https://github.com/millermatt" \
    com.docker.extension.additional-urls='[{"title":"Documentation","url":"https://github.com/imdsutil/barnacle-imds-proxy"}, {"title":"License","url":"https://github.com/imdsutil/barnacle-imds-proxy/blob/main/LICENSE"}]' \
    com.docker.extension.categories="networking,testing-tools" \
    com.docker.extension.changelog="https://raw.githubusercontent.com/imdsutil/barnacle-imds-proxy/refs/heads/main/CHANGELOG.md"

COPY --from=builder /backend/bin/service /
COPY docker-compose.yaml .
RUN sed -i "s|barnacle-imds-proxy-proxy:latest|${PROXY_IMAGE}|" docker-compose.yaml
COPY metadata.json .
COPY logo.svg .
COPY --from=client-builder /ui/build ui
CMD ["/service", "-socket", "/run/guest-services/backend.sock"]
