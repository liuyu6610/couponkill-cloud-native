FROM golang:1.23-alpine AS builder
WORKDIR /workspace
COPY ./ ./
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o /operator main.go

FROM gcr.io/distroless/static
COPY --from=builder /operator /operator
ENTRYPOINT ["/operator"]
