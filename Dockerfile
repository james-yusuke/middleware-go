# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.25.3
FROM golang:${GO_VERSION}-bookworm AS dependencies

ARG PPROF_VERSION=v0.0.0-20260709232956-b9395ee17fa0

WORKDIR /workspace
ENV CGO_ENABLED=1 \
    GOTOOLCHAIN=local

RUN GOBIN=/usr/local/bin go install github.com/google/pprof@${PPROF_VERSION}

# Keep the dependency layers stable while source files change.
COPY go.mod ./go.mod
RUN go mod download

COPY benchmarks/go.mod benchmarks/go.sum ./benchmarks/
# Dependencies intentionally remain in the image. Compose disables networking
# while measuring so a run can never download modules or add network jitter.
RUN cd benchmarks && go mod download

FROM dependencies AS lab
COPY . .

# Compose uses this stage so every service runs against one identical toolchain.
CMD ["go", "test", "./..."]

FROM lab AS test
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go test ./...
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    cd benchmarks && go test ./...

FROM lab AS race
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go test -race ./...
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    cd benchmarks && go test -race ./...

FROM lab AS benchmark-smoke
ENV ROUTER_BENCH_SIZES=100 \
    ROUTER_NATIVE_SIZES=100 \
    ROUTER_PARALLEL_SIZES=100 \
    ROUTER_FIBER_SIZES=10 \
    ROUTER_BUILD_SIZES=100
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go test -run '^$' -bench . -benchtime=5x ./middleware
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    cd benchmarks && go test -run '^$' -bench . -benchtime=5x ./...
