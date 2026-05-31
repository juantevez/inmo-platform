# --- Stage 1: Constructor ---
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.work go.work.su[m] ./

COPY shared/go.mo[d] shared/go.su[m] ./shared/
COPY contexts/auth-identity/go.mo[d] contexts/auth-identity/go.su[m] ./contexts/auth-identity/
COPY contexts/catalog/go.mo[d] contexts/catalog/go.su[m] ./contexts/catalog/
COPY contexts/contracts/go.mo[d] contexts/contracts/go.su[m] ./contexts/contracts/
COPY contexts/crm/go.mo[d] contexts/crm/go.su[m] ./contexts/crm/
COPY contexts/finances/go.mo[d] contexts/finances/go.su[m] ./contexts/finances/
COPY contexts/maintenance/go.mo[d] contexts/maintenance/go.su[m] ./contexts/maintenance/

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o contracts ./contexts/contracts/cmd/api/main.go

# --- Stage 2: Imagen de Ejecución ---
FROM alpine:3.19

WORKDIR /app

COPY --from=builder /app/contracts .
COPY --from=builder /app/contexts/contracts/migrations ./migrations

EXPOSE 8085

CMD ["./contracts"]