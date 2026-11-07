# syntax=docker/dockerfile:1
#
# CouponKill 五个 Go 服务共用的多阶段 Dockerfile（构建上下文 = 仓库根目录）。
#
#   docker build -f build/Dockerfile --build-arg SERVICE=order --build-arg PORT=8082 -t couponkill-order .
#
# SERVICE  必填：gateway | user | coupon | order | connector（对应 ./cmd/<SERVICE>）。
# PORT     仅用于 EXPOSE 元数据：gateway=8088 user=8081 coupon=8080 order=8082 connector=8085。
# 运行时配置全部来自 CK_* 环境变量（见 docs/GO-REWRITE-DESIGN.md §3）；探针由 K8s 承担
# （/healthz 存活、/readyz 就绪），所以镜像里没有 HEALTHCHECK，也没有可以执行的 shell。

ARG GO_VERSION=1.27

# ---- 构建阶段 --------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build

ARG SERVICE
ARG TARGETOS
ARG TARGETARCH

# 禁止构建时下载别的工具链：go.mod 的 go 版本必须与基础镜像一致。
ENV GOTOOLCHAIN=local \
    CGO_ENABLED=0 \
    GOFLAGS=-mod=readonly

WORKDIR /src

# 先只拷依赖清单，源码变化不会使依赖层失效；模块缓存用 BuildKit cache mount 跨构建复用。
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY cmd/ cmd/
COPY internal/ internal/

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    set -eu; \
    case "${SERVICE:-}" in \
      gateway|user|coupon|order|connector) ;; \
      *) echo "build-arg SERVICE 必须是 gateway|user|coupon|order|connector，当前为 '${SERVICE:-}'" >&2; exit 1 ;; \
    esac; \
    GOOS="${TARGETOS:-linux}" GOARCH="${TARGETARCH:-$(go env GOARCH)}" \
      go build -trimpath -ldflags "-s -w" -o /out/service "./cmd/${SERVICE}"

# ---- 运行阶段 --------------------------------------------------------------
# distroless/static：只有 CA 证书、tzdata 与 /etc/passwd，nonroot 用户 uid/gid = 65532。
FROM gcr.io/distroless/static:nonroot

ARG SERVICE
ARG PORT=8080
ARG VCS_REF=unknown

LABEL org.opencontainers.image.title="couponkill-${SERVICE}" \
      org.opencontainers.image.source="https://github.com/liuyu6610/couponkill-cloud-native" \
      org.opencontainers.image.revision="${VCS_REF}"

COPY --from=build /out/service /service

USER 65532:65532
EXPOSE ${PORT}
ENTRYPOINT ["/service"]
