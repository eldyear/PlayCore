# Шаг 1: Сборка (pure-Go — без gcc/cgo)
FROM golang:1.24-bookworm AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o easylock .

# Шаг 2: Финальный образ
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates --no-install-recommends && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /app/easylock .
RUN mkdir -p /app/data
EXPOSE 8080
CMD ["./easylock"]
