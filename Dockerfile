# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY *.go ./

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ntmonitor_gateway .

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates openssh-client

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/ntmonitor_gateway .

# Copy config template (actual config should be mounted or env vars used)
COPY config.json ./config.json

# Run the binary
CMD ["./ntmonitor_gateway"]
