# --- Stage 1: Constructor ---
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.work go.work.su[m] ./

COPY contexts/api-gateway/go.mod contexts/api-gateway/go.sum ./contexts/api-gateway/
COPY shared/go.mod shared/go.sum ./shared/
COPY contexts/auth-identity/go.mod contexts/auth-identity/go.sum ./contexts/auth-identity/
COPY contexts/catalog/go.mod contexts/catalog/go.sum ./contexts/catalog/
COPY contexts/contracts/go.mod contexts/contracts/go.sum ./contexts/contracts/
COPY contexts/crm/go.mod contexts/crm/go.sum ./contexts/crm/
COPY contexts/chat/go.mod contexts/chat/go.sum ./contexts/chat/
COPY contexts/finances/go.mod contexts/finances/go.sum ./contexts/finances/
COPY contexts/maintenance/go.mod contexts/maintenance/go.sum ./contexts/maintenance/


RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o crm ./contexts/crm/cmd/api/main.go

# --- Stage 2: Imagen de Ejecución ---
FROM alpine:3.19

WORKDIR /app

COPY --from=builder /app/crm .
COPY --from=builder /app/contexts/crm/migrations ./migrations

EXPOSE 8084

CMD ["./crm"]