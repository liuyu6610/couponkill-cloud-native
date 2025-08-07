# couponkill-go-service/Dockerfile
FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o couponkill-go-service .

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/couponkill-go-service .

EXPOSE 8090

VOLUME ["/app/logs"]

ENTRYPOINT ["./couponkill-go-service"]