# syntax=docker/dockerfile:1

# --- build stage ---
FROM golang:1.23-alpine AS build
WORKDIR /src

# Cache deps.
COPY go.mod go.sum* ./
RUN go mod download

# Build static binary.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/platform-console ./cmd/server

# --- runtime stage ---
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/platform-console /app/platform-console
COPY --from=build /src/migrations /app/migrations
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/platform-console"]
