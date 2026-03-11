# Build stage - use native platform for faster cross-compilation
FROM --platform=$BUILDPLATFORM golang:1.25-bookworm AS build

ARG TARGETARCH
ARG TARGETOS=linux

WORKDIR /src

# Copy dependency files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o /app ./cmd

FROM alpine:latest AS prod

RUN adduser -D -H appuser
USER appuser

COPY --from=build /app /usr/bin/indexer

EXPOSE 8080

ENTRYPOINT ["/usr/bin/indexer"]
CMD ["aws", "--port", "8080"]
