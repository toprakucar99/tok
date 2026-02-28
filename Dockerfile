FROM golang:1.21-alpine AS builder
WORKDIR /app
# Önce mod dosyalarını kopyalayıp paketleri indiriyoruz
COPY go.mod go.sum ./
RUN go mod download
# Sonra kodun geri kalanını kopyalıyoruz
COPY . .
RUN go build -o main .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/main .
COPY --from=builder /app/static ./static
EXPOSE 8080
CMD ["./main"]