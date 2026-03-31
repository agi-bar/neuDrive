# ---- Node stage: build the React frontend ----
FROM node:20-alpine AS frontend
WORKDIR /app/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci
COPY web/ .
RUN npm run build

# ---- Go stage: build the backend with embedded frontend ----
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Copy the built frontend into the embed directory
COPY --from=frontend /app/web/dist/ ./internal/web/dist/
RUN CGO_ENABLED=0 GOOS=linux go build -o /agenthub ./cmd/server

# ---- Final image: just the binary + migrations ----
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /agenthub .
COPY migrations/ ./migrations/
EXPOSE 8080
CMD ["./agenthub"]
