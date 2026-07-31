# Build stage
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app .

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates chromium nss freetype harfbuzz ttf-freefont
RUN if [ -x /usr/bin/chromium ] && [ ! -e /usr/bin/chromium-browser ]; then ln -s /usr/bin/chromium /usr/bin/chromium-browser; fi
ENV CHROME_BIN=/usr/bin/chromium-browser
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