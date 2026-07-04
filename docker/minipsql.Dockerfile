FROM golang:1.26-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
COPY internal/ ./internal/
RUN go build -o /out/minipsql .

FROM debian:bookworm-slim

COPY --from=build /out/minipsql /usr/local/bin/minipsql

ENTRYPOINT ["minipsql"]
