# --- Stage 1: Constructor ---
FROM golang:1.25-alpine AS builder

WORKDIR /app

# 1. Copiamos la configuración del Workspace raíz
COPY go.work go.work.su[m] ./

# 2. Copiamos todos los go.mod y go.sum del monorrepo (estrategia antipánico para go.work)
COPY contexts/api-gateway/go.mod contexts/api-gateway/go.sum ./contexts/api-gateway/
COPY shared/go.mod shared/go.sum ./shared/
COPY contexts/auth-identity/go.mod contexts/auth-identity/go.sum ./contexts/auth-identity/
COPY contexts/catalog/go.mod contexts/catalog/go.sum ./contexts/catalog/
COPY contexts/contracts/go.mod contexts/contracts/go.sum ./contexts/contracts/
COPY contexts/crm/go.mod contexts/crm/go.sum ./contexts/crm/
COPY contexts/finances/go.mod contexts/finances/go.sum ./contexts/finances/
COPY contexts/maintenance/go.mod contexts/maintenance/go.sum ./contexts/maintenance/


# Ahora Go puede validar todo el árbol del workspace y descargar en paz
RUN go mod download

# 3. Copiamos todo el código fuente real
COPY . .

# 4. Compilamos el binario de Catálogo
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o catalog ./contexts/catalog/cmd/api/main.go

# --- Stage 2: Imagen de Ejecución ---
FROM alpine:3.19

WORKDIR /app

COPY --from=builder /app/catalog .
COPY --from=builder /app/contexts/catalog/migrations ./migrations

EXPOSE 8081

CMD ["./catalog"]