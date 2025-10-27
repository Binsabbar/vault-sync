# Start with the official Go image for building the application
FROM golang:1.24 AS builder

# Set the working directory inside the container
WORKDIR /app/vault-sync

# Copy the Go modules manifests
COPY . /app/vault-sync/

RUN go mod verify
RUN go mod tidy

# Build the Go application
RUN CGO_ENABLED=0 GOOS=linux go build -o vault-sync

# Use a minimal base image for the final container
FROM debian:stable-slim


# upgrade and update the system
RUN apt-get update && apt-get upgrade -y && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

# create a non-root user and switch to it
# create a non-root group and user to run the application set it to 1001
RUN groupadd -g 1001 vaultsyncuser && useradd -r -u 1001 -g vaultsyncuser vaultsyncuser

USER vaultsyncuser

WORKDIR /app/

# Copy the built binary from the builder stage
COPY --from=builder /app/vault-sync .

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD /app/vault-sync --version || exit 1

# Set the entrypoint to the Go application
ENTRYPOINT ["/app/vault-sync"]