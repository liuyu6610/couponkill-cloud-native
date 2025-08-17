# 构建阶段：使用 Go 1.23 匹配 go.mod 版本
FROM golang:1.23-alpine AS builder
WORKDIR /app

# 复制依赖文件并下载（利用缓存）
COPY couponkill-go-service/go.mod couponkill-go-service/go.sum ./
RUN go mod download

# 复制源代码
COPY couponkill-go-service/ .

# 编译（静态链接，适应 alpine）
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o seckill-go ./cmd/server

# 运行阶段：使用最新稳定版 alpine（避免 3.18 可能的仓库问题）
FROM alpine:latest
WORKDIR /app

# 添加时区支持（可选）
RUN apk --no-cache add tzdata

# 复制二进制文件
COPY --from=builder /app/seckill-go .

EXPOSE 8090
ENTRYPOINT ["./seckill-go"]