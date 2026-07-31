FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/app .

FROM alpine:3.20

RUN apk add --no-cache \
      ca-certificates \
      tzdata \
      chromium \
      nss \
      freetype \
      harfbuzz \
      ttf-freefont \
      font-noto \
      font-noto-emoji \
 && (test -e /usr/bin/chromium-browser || ln -s /usr/bin/chromium /usr/bin/chromium-browser)

RUN addgroup -S app && adduser -S -G app -h /app app

WORKDIR /app

COPY --from=builder /out/app ./app
COPY --from=builder /src/static ./static
COPY --from=builder /src/templates ./templates

RUN mkdir -p /app/data /app/media /app/.chromium && chown -R app:app /app

USER app

ENV HOME=/app \
    PORT=8080 \
    DATA_PATH=/app/data \
    MEDIA_PATH=/app/media \
    STATIC_PATH=/app/static \
    CHROME_BIN=/usr/bin/chromium-browser \
    XDG_CONFIG_HOME=/app/.chromium \
    XDG_CACHE_HOME=/app/.chromium

EXPOSE 8080

CMD ["./app"]