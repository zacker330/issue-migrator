FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o issue-migrator .

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/issue-migrator .
# Create empty .env file if .env.example doesn't exist
RUN touch .env

EXPOSE 8080

# Set environment variable to disable output buffering
ENV GODEBUG=gctrace=0
ENV GIN_MODE=release
ENV DOCKER_CONTAINER=true

# Run with unbuffered output
CMD ["./issue-migrator"]