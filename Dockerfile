# syntax=docker/dockerfile:1.7

# ---- Builder ----
FROM --platform=$BUILDPLATFORM golang:1.21-bookworm AS builder
WORKDIR /src

# Enable Go module caching early
COPY go.mod ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy the rest of the source
COPY . .

# Build static binary with version, using pure-Go os/user to work in containers
ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=dev
ENV CGO_ENABLED=0
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -tags osusergo -o /out/github-copilot-svcs .

# Seed a config directory with correct ownership to initialize named volumes later
RUN mkdir -p /out/seed/home/nonroot/.local/share/github-copilot-svcs


# ---- Runtime ----
# Using distroless base (includes CA certs and /etc/passwd with nonroot user)
FROM gcr.io/distroless/base-debian12:nonroot AS runtime

ENV HOME=/home/nonroot
# Pre-create and copy seeded config directory so first-time volumes inherit nonroot perms
WORKDIR /home/nonroot
COPY --from=builder --chown=nonroot:nonroot /out/seed/home/nonroot/.local/share/github-copilot-svcs /home/nonroot/.local/share/github-copilot-svcs

# Persist config and tokens here (named volume recommended)
VOLUME ["/home/nonroot/.local/share/github-copilot-svcs"]

EXPOSE 8081

COPY --from=builder /out/github-copilot-svcs /usr/local/bin/github-copilot-svcs

USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/github-copilot-svcs"]
CMD ["run"]


