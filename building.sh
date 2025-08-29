#!/bin/bash
# 阿里云镜像仓库配置
REGISTRY="crpi-n5rumpjwbqinoz4c.cn-hangzhou.personal.cr.aliyuncs.com/thetestspacefordocker/my-docker"
CANARY_REGISTRY="crpi-n5rumpjwbqinoz4c-vpc.cn-hangzhou.personal.cr.aliyuncs.com/thetestspacefordocker/canary-keda-dev"

echo "Building couponkill-user-service..."
docker build -f couponkill-user-service/Dockerfile -t couponkill-user-service .

echo "Building couponkill-coupon-service..."
docker build -f couponkill-coupon-service/Dockerfile -t couponkill-coupon-service .

echo "Building couponkill-order-service..."
docker build -f couponkill-order-service/Dockerfile -t couponkill-order-service .

echo "Building couponkill-gateway..."
docker build -f couponkill-gateway/Dockerfile -t couponkill-gateway .

echo "Building couponkill-go-service..."
docker build -f couponkill-go-service/Dockerfile -t couponkill-go-service .

echo "Building operator-coupon..."
docker build -f operator-coupon/Dockerfile -t operator-coupon .


#Linux/Mac shell!/bin/bash

echo "Building couponkill-user-service..."
docker build -f couponkill-user-service/Dockerfile -t ${REGISTRY}/user:latest .

echo "Building couponkill-coupon-service..."
docker build -f couponkill-coupon-service/Dockerfile -t ${REGISTRY}/coupon:latest .

echo "Building couponkill-order-service..."
docker build -f couponkill-order-service/Dockerfile -t ${REGISTRY}/order:latest .

echo "Building couponkill-gateway..."
docker build -f couponkill-gateway/Dockerfile -t ${REGISTRY}/gateway:latest .

echo "Building couponkill-go-service..."
docker build -f couponkill-go-service/Dockerfile -t ${REGISTRY}/seckill-go:latest .

echo "Building operator-coupon..."
docker build -f operator-coupon/Dockerfile -t ${REGISTRY}/operator:latest .

echo "All images built successfully!"

# 推送稳定版镜像到阿里云镜像仓库
echo "Pushing stable images to registry..."
echo "Please ensure you are logged in to the registry before proceeding."
docker push ${REGISTRY}/user:latest
docker push ${REGISTRY}/coupon:latest
docker push ${REGISTRY}/order:latest
docker push ${REGISTRY}/gateway:latest
docker push ${REGISTRY}/seckill-go:latest
docker push ${REGISTRY}/operator:latest

echo "All stable images pushed successfully!"

# 构建金丝雀版本镜像
echo "Building canary images..."
docker build -f couponkill-user-service/Dockerfile -t ${CANARY_REGISTRY}/user:canary .
docker build -f couponkill-coupon-service/Dockerfile -t ${CANARY_REGISTRY}/coupon:canary .
docker build -f couponkill-order-service/Dockerfile -t ${CANARY_REGISTRY}/order:canary .
docker build -f couponkill-gateway/Dockerfile -t ${CANARY_REGISTRY}/gateway:canary .
docker build -f couponkill-go-service/Dockerfile -t ${CANARY_REGISTRY}/seckill-go:canary .
docker build -f operator-coupon/Dockerfile -t ${CANARY_REGISTRY}/operator:canary .

echo "All canary images built successfully!"

# 推送金丝雀版本镜像到阿里云镜像仓库
echo "Pushing canary images to registry..."
echo "Please ensure you are logged in to the registry before proceeding."
docker push ${CANARY_REGISTRY}/user:canary
docker push ${CANARY_REGISTRY}/coupon:canary
docker push ${CANARY_REGISTRY}/order:canary
docker push ${CANARY_REGISTRY}/gateway:canary
docker push ${CANARY_REGISTRY}/seckill-go:canary
docker push ${CANARY_REGISTRY}/operator:canary

echo "All canary images pushed successfully!"

# 拉取并推送依赖镜像到阿里云镜像仓库
echo "Pulling and pushing dependency images..."
# MySQL
docker pull mysql:8.0
docker tag mysql:8.0 ${REGISTRY}/mysql:8.0
docker push ${REGISTRY}/mysql:8.0

# Redis
docker pull redis:7.0
docker tag redis:7.0 ${REGISTRY}/redis:7.0
docker push ${REGISTRY}/redis:7.0

# RocketMQ nameserver
docker pull apache/rocketmq:4.9.4-alpine
docker tag apache/rocketmq:4.9.4-alpine ${REGISTRY}/rocketmq-namesrv:4.9.4-alpine
docker push ${REGISTRY}/rocketmq-namesrv:4.9.4-alpine

# RocketMQ broker
docker pull apache/rocketmq:4.9.4-alpine
docker tag apache/rocketmq:4.9.4-alpine ${REGISTRY}/rocketmq-broker:4.9.4-alpine
docker push ${REGISTRY}/rocketmq-broker:4.9.4-alpine

# Nacos
docker pull nacos/nacos-server:v2.2.3
docker tag nacos/nacos-server:v2.2.3 ${REGISTRY}/nacos-server:v2.2.3
docker push ${REGISTRY}/nacos-server:v2.2.3

# Sentinel
docker pull bladex/sentinel-dashboard:1.8.6
docker tag bladex/sentinel-dashboard:1.8.6 ${REGISTRY}/sentinel-dashboard:1.8.6
docker push ${REGISTRY}/sentinel-dashboard:1.8.6

# Kafka
docker pull bitnami/kafka:3.4.0
docker tag bitnami/kafka:3.4.0 ${REGISTRY}/kafka:3.4.0
docker push ${REGISTRY}/kafka:3.4.0

# Zookeeper (for Kafka)
docker pull bitnami/zookeeper:3.8.1
docker tag bitnami/zookeeper:3.8.1 ${REGISTRY}/zookeeper:3.8.1
docker push ${REGISTRY}/zookeeper:3.8.1

echo "All dependency images pulled and pushed successfully!"
echo "All images built and pushed successfully!"