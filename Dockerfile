FROM --platform=$BUILDPLATFORM golang:1.23.7 AS builder


WORKDIR /app

# Copy go.mod and go.sum first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

ARG TARGETOS TARGETARCH

# Build the application
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o nodewatcher ./cmd/nodewatcher

# Use a small alpine image for the final container
FROM alpine:3.18

RUN apk --no-cache add ca-certificates tzdata && \
    update-ca-certificates

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/nodewatcher .

# Set the entrypoint
ENTRYPOINT ["/app/nodewatcher"] 