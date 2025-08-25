# 多阶段构建Docker镜像，第一阶段为Go应用编译阶段
# 使用golang:1.24作为基础镜像，命名为builder阶段
FROM golang:1.24-alpine AS builder

# 安装git，某些Go依赖可能需要
RUN apk add --no-cache git

# 设置工作目录为/workspace
WORKDIR /workspace

# 将当前目录下的所有文件复制到容器的工作目录中
COPY operator-coupon/ .

# 检查文件是否存在
RUN ls -la

# 整理Go模块依赖，确保依赖关系正确
RUN go mod tidy

# 编译Go应用程序，生成名为operator的可执行文件
# 禁用CGO，设置目标操作系统为Linux
RUN CGO_ENABLED=0 GOOS=linux go build -o /operator .

# 第二阶段构建最小化运行时镜像
# 使用distroless/static作为基础镜像，该镜像只包含最基本的运行环境
FROM gcr.io/distroless/static

# 从builder阶段复制编译好的可执行文件到当前镜像中
COPY --from=builder /couponkill-operator /operator

# 设置容器启动时执行的命令，运行operator可执行文件
ENTRYPOINT ["/operator"]
