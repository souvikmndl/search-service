FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/search-api ./cmd/search

RUN go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

FROM alpine:3.22

RUN addgroup -S app && adduser -S app -G app

WORKDIR /app
COPY --from=builder /app/search-api /app/search-api
COPY --from=builder /app/migrations /app/migrations
COPY --from=builder /go/bin/migrate /usr/local/bin/migrate
COPY entrypoint.sh /entrypoint.sh

RUN chmod +x /entrypoint.sh && \
    mkdir -p /var/log/search_service && \
    chown -R app:app /var/log/search_service

USER app

EXPOSE 3000

ENTRYPOINT ["/entrypoint.sh"]
