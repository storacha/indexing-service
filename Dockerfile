FROM golang:1.23-bullseye AS build

WORKDIR /indexing-service

COPY go.* .
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 go build -o indexer ./cmd

FROM scratch
COPY --from=build /indexing-service/indexer /usr/bin/

EXPOSE 8080

ENTRYPOINT ["/usr/bin/indexer"]
CMD ["aws", "--port", "8080"]
