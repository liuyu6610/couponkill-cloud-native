#!/bin/bash
# 部署命名空间
kubectl apply -f namespace.yaml

# 部署中间件（如需，如Redis、Kafka，需补充对应YAML）
# kubectl apply -f middleware/
# 部署指定服务（参数为服务名，如order、coupon）
deploy_service() {
  if [ -d "services/$1" ]; then
    echo "部署服务: $1"
    kubectl apply -f services/$1/
  else
    echo "服务 $1 不存在"
  fi
}
# 部署网络配置
deploy_network() {
  echo "部署网络配置"
  kubectl apply -f network/
}

# 部署Operator
deploy_operator() {
  echo "部署Operator CRD"
  kubectl apply -f operator/crd.yaml
  echo "部署Operator控制器"
  kubectl apply -f operator/deployment.yaml
}

# 示例：部署订单服务
# deploy_service order

# 示例：部署所有服务
# for service in coupon order user go-service; do deploy_service $service; done
# deploy_network
# deploy_operator