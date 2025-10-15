# Build stage
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app .

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates
RUN adduser -D -s /bin/sh appuser

WORKDIR /root/

COPY --from=builder /app/app .
COPY --from=builder /app/static ./static
COPY --from=builder /app/templates ./templates

RUN mkdir -p /app/data && chown appuser:appuser /app/data

USER appuser

EXPOSE 8080

ENV PORT=8080
ENV DATA_PATH=/app/data

CMD ["./app"]