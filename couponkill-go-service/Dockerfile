# couponkill-go-service/Dockerfile
FROM golang:1.23-alpine AS builder

WORKDIR /app

# 先复制依赖文件，充分利用Docker缓存
COPY go.mod go.sum ./
RUN go mod download

# 再复制源代码
COPY . .

# 构建应用
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o couponkill-go-service .

# 使用固定版本的基础镜像
FROM alpine:3.18

WORKDIR /app

# 从构建阶段复制可执行文件
COPY --from=builder /app/couponkill-go-service .

# 暴露端口
EXPOSE 8090

# 创建日志目录并设置卷
VOLUME ["/app/logs"]

# 添加健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 CMD wget --quiet --tries=1 --spider http://localhost:8090/health || exit 1

# 启动应用
ENTRYPOINT ["./couponkill-go-service"]
