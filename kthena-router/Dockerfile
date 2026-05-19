FROM golang:1.24 AS builder

WORKDIR /workspace

# Copy entire project context to resolve internal dependencies
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY . .

WORKDIR /workspace/bench/kthena-router
RUN CGO_ENABLED=0 GOOS=linux go build -a -o kthena-router main.go

FROM alpine:latest
WORKDIR /
COPY --from=builder /workspace/bench/kthena-router/kthena-router .
COPY --from=builder /workspace/bench/kthena-router/router-config.yaml .
EXPOSE 8080
ENTRYPOINT ["/kthena-router"]
