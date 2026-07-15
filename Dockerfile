# Build the KelmaSync server as a small static binary.
FROM golang:1.26 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

# Copy only server source. Runtime data, clients, Git metadata, and environment
# files are excluded both here and by .dockerignore.
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/server /app/server
COPY --from=build /src/migrations /app/migrations
ENV MIGRATIONS_DIR=/app/migrations
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/server"]
