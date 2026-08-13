# syntax=docker/dockerfile:1
#
# Builds the image from source. Dockerfile.release packages the binaries
# GoReleaser has already built; both produce the same runtime, and the build
# below matches the release build so that what CI smoke tests is what ships:
# same Go version as go.mod, same tags, and the same static link.
FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
# Momobase links SQLite through cgo. A CGO_ENABLED=0 binary compiles but aborts
# on its first query, so cgo stays on and the C runtime is linked statically.
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN CGO_ENABLED=1 GOOS=linux go build \
    -trimpath \
    -tags "osusergo,netgo,sqlite_omit_load_extension" \
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
