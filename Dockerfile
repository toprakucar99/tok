FROM golang:1.21-alpine

WORKDIR /app

# Sadece mod dosyasını kopyala
COPY go.mod ./

# go.sum dosyasını ve eksik bağımlılıkları Docker içinde baştan oluştur
RUN go mod tidy

# Şimdi diğer dosyaları kopyala
COPY . .

# Tekrar bir kontrol yap ve derle
RUN go mod tidy
RUN go build -o main .

EXPOSE 8080

CMD ["./main"]