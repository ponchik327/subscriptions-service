FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/app ./cmd/app

# ── runtime ──────────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static:nonroot

COPY --from=builder /bin/app /app
COPY --from=builder /app/migrations /migrations
COPY --from=builder /app/config /config

EXPOSE 8080

ENTRYPOINT ["/app"]
