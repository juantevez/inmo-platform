# --- Stage 1: Constructor ---
FROM golang:1.25-alpine AS builder

WORKDIR /app

# 1. Copiamos la configuración del Workspace raíz
COPY go.work go.work.su[m] ./

# 2. Copiamos todos los go.mod y go.sum del monorrepo (estrategia antipánico para go.work)
COPY shared/go.mo[d] shared/go.su[m] ./shared/
COPY contexts/auth-identity/go.mo[d] contexts/auth-identity/go.su[m] ./contexts/auth-identity/
COPY contexts/catalog/go.mo[d] contexts/catalog/go.su[m] ./contexts/catalog/
COPY contexts/contracts/go.mo[d] contexts/contracts/go.su[m] ./contexts/contracts/
COPY contexts/crm/go.mo[d] contexts/crm/go.su[m] ./contexts/crm/
COPY contexts/finances/go.mo[d] contexts/finances/go.su[m] ./contexts/finances/
COPY contexts/maintenance/go.mo[d] contexts/maintenance/go.su[m] ./contexts/maintenance/

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