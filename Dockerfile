# Build stage
FROM golang:1.26-alpine AS builder

# Instalar git y certificados raíz (necesario para HTTPS y descargas de módulos)
RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Copiar archivos de dependencias
COPY go.mod go.sum ./
RUN go mod download

# Copiar el código fuente
COPY . .

# Compilar la aplicación
# CGO_ENABLED=0 asegura un binario estático linkado
RUN CGO_ENABLED=0 GOOS=linux go build -o acortador .

# Run stage
FROM alpine:latest

WORKDIR /app

# Copiar el binario desde el builder
COPY --from=builder /app/acortador .

# Copiar archivos estáticos
COPY --from=builder /app/static ./static

# Instalar certificados CA en la imagen final para conexiones HTTPS seguras (Supabase)
RUN apk add --no-cache ca-certificates

# Exponer el puerto
EXPOSE 8080

# Configurar variables de entorno por defecto
ENV PORT=8080
# DATABASE_URL debe ser provisto en tiempo de ejecución (secrets)

# Punto de entrada
CMD ["./acortador"]
