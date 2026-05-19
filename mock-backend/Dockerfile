FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY . .
RUN go mod init mock-backend || true
RUN go mod tidy || true
RUN CGO_ENABLED=0 GOOS=linux go build -o mock-backend main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/mock-backend .
EXPOSE 8080
CMD ["./mock-backend"]
