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
COPY --from=builder /app/.env.example .env

EXPOSE 8080

CMD ["./issue-migrator"]