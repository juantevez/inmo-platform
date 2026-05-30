# --- Stage 1: Constructor ---
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copiamos los archivos de configuración de módulos
COPY go.mod go.sum ./
RUN go mod download

# Copiamos el código fuente del contexto
COPY . .

# Compilamos el binario optimizado para producción
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o auth-api ./cmd/api/main.go

# --- Stage 2: Imagen de Ejecución ---
FROM alpine:3.19

WORKDIR /app

# Copiamos el binario desde el stage anterior
COPY --from=builder /app/auth-api .
# Copiamos la carpeta de migraciones
COPY --from=builder /app/migrations ./migrations

EXPOSE 8080

CMD ["./auth-api"]