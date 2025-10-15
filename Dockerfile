# Start with the official Go image for building the application
FROM golang:1.25 as builder

# Set the working directory inside the container
WORKDIR /app/vault-sync

# Copy the Go modules manifests
COPY . /app/vault-sync/

RUN go mod verify && go mod tidy

# Build the Go application
RUN CGO_ENABLED=0 GOOS=linux go build -o vault-sync

# Use a minimal base image for the final container
FROM debian:stable-slim

# Set the working directory inside the container

# upgrade and update the system
RUN apt-get update && apt-get upgrade -y && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

# create a non-root user and switch to it
RUN useradd -m vaultsyncuser
USER vaultsyncuser
WORKDIR /app/vault-sync

# Copy the built binary from the builder stage
COPY --from=builder /app/vault-sync/vault-sync .

# Set the entrypoint to the Go application
ENTRYPOINT ["./vault-sync"]