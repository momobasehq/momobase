# syntax=docker/dockerfile:1
#
# Builds the image from source. Dockerfile.release packages the binaries
# GoReleaser has already built; both produce the same runtime, and the build
# below matches the release build so that what CI smoke tests is what ships:
# same Go version as go.mod, same tags, and the same static link.
# The dashboard bundle. Nothing under web/dashboard/dist is committed, so the image
# builds it here and the Go stage compiles with -tags dashboard to embed it.
FROM node:22-bookworm-slim AS web
WORKDIR /web
RUN npm install --global pnpm@11
# Manifests first, so a source-only change reuses the installed dependency layer.
COPY web/pnpm-workspace.yaml web/pnpm-lock.yaml web/package.json ./
COPY web/sdk/package.json ./sdk/
COPY web/dashboard/package.json ./dashboard/
RUN pnpm install --frozen-lockfile
COPY web ./
# Recursive, not filtered: the dashboard imports @momobase/sdk through the workspace
# and resolves its types from dist, so the SDK has to be built first. pnpm orders the
# two by their dependency edge.
RUN pnpm --recursive run build

FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
# After COPY . ., because .dockerignore excludes web/dashboard/dist — copying earlier
# would let the source copy delete the bundle again.
COPY --from=web /web/dashboard/dist ./web/dashboard/dist
# Momobase links SQLite through cgo. A CGO_ENABLED=0 binary compiles but aborts
# on its first query, so cgo stays on and the C runtime is linked statically.
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN CGO_ENABLED=1 GOOS=linux go build \
    -trimpath \
    -tags "osusergo,netgo,sqlite_omit_load_extension,dashboard" \
    -ldflags="-s -w -extldflags \"-static\" -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o /out/momobase ./cmd/momobase

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/momobase /usr/local/bin/momobase
RUN mkdir -p /app/data && useradd -r -u 10001 -g nogroup momobase && chown -R momobase:nogroup /app
USER momobase
EXPOSE 9090
ENTRYPOINT ["momobase"]
CMD ["serve"]
