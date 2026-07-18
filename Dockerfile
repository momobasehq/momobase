# syntax=docker/dockerfile:1
FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/momobase ./cmd/momobase

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/momobase /usr/local/bin/momobase
COPY web ./web
RUN mkdir -p /app/data && useradd -r -u 10001 -g nogroup momobase && chown -R momobase:nogroup /app
USER momobase
EXPOSE 9090
ENTRYPOINT ["momobase"]
CMD ["serve"]
