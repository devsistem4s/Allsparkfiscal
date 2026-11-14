FROM golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /allspark ./cmd/api
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /allspark /allspark
COPY migrations /app/migrations
USER 1000:1000
ENTRYPOINT ["/allspark"]
