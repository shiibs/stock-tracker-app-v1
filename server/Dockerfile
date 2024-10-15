# Build the Go app
FROM golang:1.22.6-alpine AS builder

# Install required dependencies
RUN apk update && apk add --no-cache git

# Set the current working directory inside the container
WORKDIR /app

# Copy the Go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the entire source code
COPY . .

# Build the Go application
RUN go build -o main .

# Run the Go app in a minimal image
FROM alpine:latest

# Install CA certificates for HTTPS requests
RUN apk add --no-cache ca-certificates

# Set the working directory in the container
WORKDIR /root/

# Copy the Go app binary from the builder stage
COPY --from=builder /app/main .

# Expose the port your application listens
EXPOSE 8080

# Command to run the Go application
CMD ["./main"]

