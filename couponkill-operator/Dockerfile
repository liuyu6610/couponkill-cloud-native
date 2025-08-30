# Build the manager binary
FROM golang:1.23-alpine AS builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY couponkill-operator/go.mod go.mod
COPY couponkill-operator/go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY couponkill-operator/main.go main.go
COPY couponkill-operator/api/ api/
COPY couponkill-operator/controllers/ controllers/
# Copy hack directory contents if it exists
COPY couponkill-operator/hack/ hack/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .

# 设置Go运行时优化参数
ENV GOGC=20
ENV GOMAXPROCS=4

USER 65532:65532

# 健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 CMD ["/manager", "--help"]

ENTRYPOINT ["/manager"]