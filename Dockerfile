# Multi-stage build for murlog (serve mode).

# --- Stage 1: Build SPA ---
FROM node:24-alpine AS web
WORKDIR /app
COPY version.txt ./
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# --- Stage 2: Build Go binary ---
FROM golang:1.26-alpine AS go
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /app/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -o murlog ./cmd/murlog

# --- Stage 3: Runtime ---
FROM alpine:3.21
RUN apk add --no-cache ca-certificates curl tzdata
WORKDIR /app

COPY --from=go /app/murlog ./murlog
COPY --from=web /app/web/dist ./web/dist

EXPOSE 8080
CMD ["./murlog", "serve"]
