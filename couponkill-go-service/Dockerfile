# couponkill-go-service/Dockerfile
FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod .
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o seckill .

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/seckill .

VOLUME /app/logs

EXPOSE 8090

ENTRYPOINT ["./seckill"]