# Build stage
FROM golang:1.25.1-alpine AS builder
WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build API binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/api ./api

# Runtime stage — minimal image
FROM alpine:3.19
RUN apk --no-cache add ca-certificates wget
COPY --from=builder /bin/api /api

# Non-root user
USER nobody

EXPOSE 8080
ENTRYPOINT ["/api"]
