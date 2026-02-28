FROM golang:1.21-alpine AS builder
WORKDIR /app

# Paket listesini kopyala
COPY go.mod ./
# go.sum hatasını bypass etmek ve eksikleri çekmek için tidy çalıştırıyoruz
COPY . .
RUN go mod tidy
RUN go build -o main .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/main .
COPY --from=builder /app/static ./static
EXPOSE 8080
CMD ["./main"]