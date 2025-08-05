FROM golang:1.24-bookworm AS build

WORKDIR /indexing-service

COPY go.* .
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-w -s" -o indexer ./cmd

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /indexing-service/indexer /usr/bin/

EXPOSE 8080

ENTRYPOINT ["/usr/bin/indexer"]
CMD ["aws", "--port", "8080"]
