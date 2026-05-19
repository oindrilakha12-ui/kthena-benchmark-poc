FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY . .
RUN go mod init loadgen || true
RUN go mod tidy || true
RUN CGO_ENABLED=0 GOOS=linux go build -o loadgen main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/loadgen .
ENTRYPOINT ["./loadgen"]
