# 构建阶段：使用 golang:1.23-alpine 作为基础镜像
FROM golang:1.23-alpine AS builder

# 安装 git（某些依赖可能需要）
RUN apk add --no-cache git

# 设置工作目录
WORKDIR /app

# 复制 go mod 和 sum 文件
COPY couponkill-go-service/go.mod couponkill-go-service/go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY couponkill-go-service/ .

# 编译（静态链接，适应 alpine）
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o seckill-go ./cmd/server

# 运行阶段：使用最新稳定版 alpine（避免 3.18 可能的仓库问题）
FROM alpine:latest

# 安装 ca-certificates（某些 HTTPS 请求可能需要）
RUN apk add --no-cache ca-certificates

# 创建非 root 用户（安全最佳实践）
RUN adduser -D -s /bin/sh seckill

# 设置工作目录
WORKDIR /app

# 从构建阶段复制编译好的二进制文件
COPY --from=builder /app/seckill-go .

# 更改二进制文件所有权（安全最佳实践）
RUN chown seckill:seckill seckill-go

# 切换到非 root 用户
USER seckill

# 暴露端口
EXPOSE 8090

# 启动应用
ENTRYPOINT ["./seckill-go"]
