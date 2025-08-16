# 构建阶段
FROM golang:1.23-alpine AS builder
WORKDIR /app
# 复制go模块文件
COPY couponkill-go-service/go.mod .
COPY couponkill-go-service/go.sum .
# 下载依赖
RUN go mod download

# 复制源码
COPY couponkill-go-service/ .

# 编译（静态链接，适应alpine）
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o seckill-go ./cmd/server

# 运行阶段
FROM alpine:3.18
WORKDIR /app
# 复制二进制文件
COPY --from=builder /app/seckill-go .

# 暴露端口（与部署文件一致）
EXPOSE 8082

# 启动
ENTRYPOINT ["./seckill-go"]