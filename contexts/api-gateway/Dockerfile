# --- Stage 1: Constructor ---
FROM golang:1.25-alpine AS builder

WORKDIR /app

# 1. Copiamos la configuración del Workspace raíz
COPY go.work go.work.su[m] ./

# 2. Copiamos todos los go.mod y go.sum del monorrepo para que go.work no rompa
COPY shared/go.mo[d] shared/go.su[m] ./shared/
COPY contexts/auth-identity/go.mo[d] contexts/auth-identity/go.su[m] ./contexts/auth-identity/
COPY contexts/catalog/go.mo[d] contexts/catalog/go.su[m] ./contexts/catalog/
COPY contexts/contracts/go.mo[d] contexts/contracts/go.su[m] ./contexts/contracts/
COPY contexts/crm/go.mo[d] contexts/crm/go.su[m] ./contexts/crm/
COPY contexts/finances/go.mo[d] contexts/finances/go.su[m] ./contexts/finances/
COPY contexts/maintenance/go.mo[d] contexts/maintenance/go.su[m] ./contexts/maintenance/
# 🔥 Agregamos la línea para el api-gateway:
COPY contexts/api-gateway/go.mo[d] contexts/api-gateway/go.su[m] ./contexts/api-gateway/

# Descargamos dependencias de todo el ecosistema
RUN go mod download

# 3. Copiamos todo el código fuente real del monorrepo
COPY . .

# 4. Compilamos el binario del API Gateway apuntando a su main real
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o api-gateway ./contexts/api-gateway/cmd/gateway/main.go

# --- Stage 2: Imagen de Ejecución ---
FROM alpine:3.19

WORKDIR /app

RUN apk --no-cache add ca-certificates

# Copiamos el binario final
COPY --from=builder /app/api-gateway .

EXPOSE 8000

CMD ["./api-gateway"]
