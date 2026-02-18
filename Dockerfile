# Build stage
FROM golang:1.26-alpine AS builder

# Instalar dependencias necesarias para CGO (necesario para sqlite si no se usa la versión pure-go, pero nosotros usamos modernc.org/sqlite que es pure Go)
# Aún así, es buena práctica tener git.
RUN apk add --no-cache git

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

# Exponer el puerto
EXPOSE 8080

# Configurar variables de entorno por defecto
ENV PORT=8080
ENV DB_PATH=./urls.db

# Volumen para persistencia de datos
VOLUME /app/data

# Punto de entrada
CMD ["./acortador"]
