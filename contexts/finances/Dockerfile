# --- Stage 1: Constructor ---
FROM golang:1.25-alpine AS builder

WORKDIR /app

# 1. Copiamos el bloque de infraestructura compartida
COPY shared/ ./shared/

# 2. Copiamos la estructura interna de Finanzas
COPY contexts/finances/ ./contexts/finances/

# 3. 🚀 CREAMOS EL WORKSPACE EN CALIENTE (Solo con lo que copiamos)
RUN go work init ./shared ./contexts/finances

# 4. Nos paramos dentro del contexto de finanzas para descargar y compilar
WORKDIR /app/contexts/finances
RUN go mod download

# Compilamos el binario optimizado para producción
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o finances ./cmd/api/main.go

# --- Stage 2: Imagen de Ejecución ---
FROM alpine:3.19

WORKDIR /app

# Copiamos el binario y las migraciones construidos en el stage anterior
COPY --from=builder /app/contexts/finances/finances .
COPY --from=builder /app/contexts/finances/migrations ./migrations

EXPOSE 8082

CMD ["./finances"]
