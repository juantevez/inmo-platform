# --- Stage 1: Constructor ---
FROM golang:1.25-alpine AS builder

WORKDIR /app

# 1. Copiamos el bloque de infraestructura compartida
COPY shared/ ./shared/

# 2. Copiamos la estructura interna de Mantenimiento
COPY contexts/maintenance/ ./contexts/maintenance/

# 3. 🚀 Creamos el workspace temporal en caliente
RUN go work init ./shared ./contexts/maintenance

# 4. Nos paramos dentro del contexto de mantenimiento para descargar y compilar
WORKDIR /app/contexts/maintenance
RUN go mod download

# Compilamos el binario optimizado para producción
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o maintenance ./cmd/api/main.go

# --- Stage 2: Imagen de Ejecución ---
FROM alpine:3.19

WORKDIR /app

# Copiamos el binario y las migraciones
COPY --from=builder /app/contexts/maintenance/maintenance .
COPY --from=builder /app/contexts/maintenance/migrations ./migrations

EXPOSE 8083

CMD ["./maintenance"]
