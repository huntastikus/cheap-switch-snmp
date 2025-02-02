FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache gcc musl-dev

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o cheap-switch-snmp

# Final stage
FROM alpine:3.18

# Install runtime dependencies for SNMP
RUN apk add --no-cache ca-certificates net-snmp

# Copy the binary from builder
COPY --from=builder /app/cheap-switch-snmp /cheap-switch-snmp

# Expose SNMP port (default UDP 161)
EXPOSE 161/udp

# Run the application
ENTRYPOINT ["./cheap-switch-snmp"]