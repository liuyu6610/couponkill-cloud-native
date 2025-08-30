# PowerShell script to build Docker images for couponkill project

param(
    [Parameter(Position=0)]
    [string]$Command = "help"
)

$REGISTRY = "crpi-n5rumpjwbqinoz4c.cn-hangzhou.personal.cr.aliyuncs.com/thetestspacefordocker/my-docker"
$CANARY_REGISTRY = "crpi-n5rumpjwbqinoz4c.cn-hangzhou.personal.cr.aliyuncs.com/thetestspacefordocker/canary-keda-dev"

function Show-Help {
    Write-Host "Usage: .\build.ps1 <target>"
    Write-Host ""
    Write-Host "Available targets:"
    Write-Host "  help                      Display this help"
    Write-Host "  build-all-images          Build all Docker images for the project"
    Write-Host "  deploy-chart              Deploy the entire CouponKill system using Helm chart"
    Write-Host "  deploy-chart-prod         Deploy the entire CouponKill system using Helm chart in production mode"
    Write-Host "  deploy-chart-canary       Deploy the entire CouponKill system using Helm chart with canary release"
    Write-Host "  build-and-push-all        Build and push all images to registry"
    Write-Host "  build-all-images-canary   Build and tag all Docker images for canary release"
    Write-Host "  push-all-images-canary    Push all Docker images for canary release to registry"
    Write-Host "  build-and-push-all-canary Build and push all canary images to registry"
    Write-Host "  pull-dependency-images    Pull all dependency images to local registry"
    Write-Host "  build-and-push-all-complete Build and push all project and dependency images"
}

function Build-All-Images {
    Write-Host "Building all Docker images..." -ForegroundColor Green
    
    docker build -t gateway:latest -f couponkill-gateway/Dockerfile .
    docker build -t coupon:latest -f couponkill-coupon-service/Dockerfile .
    docker build -t order:latest -f couponkill-order-service/Dockerfile .
    docker build -t user:latest -f couponkill-user-service/Dockerfile .
    docker build -t seckill-go:latest -f couponkill-go-service/Dockerfile .
    docker build -t operator:latest -f couponkill-operator/Dockerfile .
    
    Write-Host "All images built successfully!" -ForegroundColor Green
}

function Deploy-Chart {
    Write-Host "Deploying CouponKill system using Helm chart..." -ForegroundColor Green
    helm upgrade --install couponkill ./charts/couponkill --namespace couponkill --create-namespace
}

function Deploy-Chart-Prod {
    Write-Host "Deploying CouponKill system using Helm chart in production mode..." -ForegroundColor Green
    helm upgrade --install couponkill ./charts/couponkill --namespace couponkill --create-namespace -f ./charts/couponkill/values-prod.yaml
}

function Deploy-Chart-Canary {
    Write-Host "Deploying CouponKill system using Helm chart with canary release..." -ForegroundColor Green
    helm upgrade --install couponkill ./charts/couponkill --namespace couponkill --create-namespace -f ./charts/couponkill/values.canary-keda.yaml
}

function Build-And-Push-All {
    Write-Host "Building and pushing all Docker images to registry..." -ForegroundColor Green
    
    # Build images with local tags
    docker build -t gateway:latest -f couponkill-gateway/Dockerfile .
    docker build -t coupon:latest -f couponkill-coupon-service/Dockerfile .
    docker build -t order:latest -f couponkill-order-service/Dockerfile .
    docker build -t user:latest -f couponkill-user-service/Dockerfile .
    docker build -t seckill-go:latest -f couponkill-go-service/Dockerfile .
    docker build -t operator:latest -f couponkill-operator/Dockerfile .
    
    # Tag images for registry (阿里云个人仓库使用标签区分镜像)
    docker tag gateway:latest "${REGISTRY}:gateway"
    docker tag coupon:latest "${REGISTRY}:coupon"
    docker tag order:latest "${REGISTRY}:order"
    docker tag user:latest "${REGISTRY}:user"
    docker tag seckill-go:latest "${REGISTRY}:seckill-go"
    docker tag operator:latest "${REGISTRY}:operator"
    
    # Push images
    docker push "${REGISTRY}:gateway"
    docker push "${REGISTRY}:coupon"
    docker push "${REGISTRY}:order"
    docker push "${REGISTRY}:user"
    docker push "${REGISTRY}:seckill-go"
    docker push "${REGISTRY}:operator"
    
    Write-Host "All images built and pushed successfully!" -ForegroundColor Green
}

function Build-All-Images-Canary {
    Write-Host "Building and tagging all Docker images for canary release..." -ForegroundColor Green
    
    docker build -t gateway:latest -f couponkill-gateway/Dockerfile .
    docker build -t coupon:latest -f couponkill-coupon-service/Dockerfile .
    docker build -t order:latest -f couponkill-order-service/Dockerfile .
    docker build -t user:latest -f couponkill-user-service/Dockerfile .
    docker build -t seckill-go:latest -f couponkill-go-service/Dockerfile .
    docker build -t operator:latest -f couponkill-operator/Dockerfile .
    
    # Tag images for canary registry (阿里云个人仓库使用标签区分镜像)
    docker tag gateway:latest "${CANARY_REGISTRY}:gateway"
    docker tag coupon:latest "${CANARY_REGISTRY}:coupon"
    docker tag order:latest "${CANARY_REGISTRY}:order"
    docker tag user:latest "${CANARY_REGISTRY}:user"
    docker tag seckill-go:latest "${CANARY_REGISTRY}:seckill-go"
    docker tag operator:latest "${CANARY_REGISTRY}:operator"
    
    Write-Host "All canary images built and tagged successfully!" -ForegroundColor Green
}

function Push-All-Images-Canary {
    Write-Host "Pushing all Docker images for canary release to registry..." -ForegroundColor Green
    
    docker push "${CANARY_REGISTRY}:gateway"
    docker push "${CANARY_REGISTRY}:coupon"
    docker push "${CANARY_REGISTRY}:order"
    docker push "${CANARY_REGISTRY}:user"
    docker push "${CANARY_REGISTRY}:seckill-go"
    docker push "${CANARY_REGISTRY}:operator"
    
    Write-Host "All canary images pushed successfully!" -ForegroundColor Green
}

function Build-And-Push-All-Canary {
    Build-All-Images-Canary
    Push-All-Images-Canary
    
    Write-Host "All canary images built and pushed successfully!" -ForegroundColor Green
}

function Pull-Dependency-Images {
    Write-Host "Pulling all dependency images to local registry..." -ForegroundColor Green
    
    # Pull and retag MySQL image
    docker pull mysql:8.0
    docker tag mysql:8.0 "${REGISTRY}/mysql"
    docker push "${REGISTRY}/mysql"
    
    # Pull and retag Redis image
    docker pull redis:7.0
    docker tag redis:7.0 "${REGISTRY}/redis"
    docker push "${REGISTRY}/redis"
    
    # Pull and retag RocketMQ nameserver image
    docker pull apache/rocketmq:5.3.1
    docker tag apache/rocketmq:5.3.1 "${REGISTRY}/rocketmq-namesrv"
    docker push "${REGISTRY}/rocketmq-namesrv"
    
    # Pull and retag RocketMQ broker image
    docker pull apache/rocketmq:5.3.1
    docker tag apache/rocketmq:5.3.1 "${REGISTRY}/rocketmq-broker"
    docker push "${REGISTRY}/rocketmq-broker"
    
    # Pull and retag Nacos image
    docker pull nacos/nacos-server:v2.2.3
    docker tag nacos/nacos-server:v2.2.3 "${REGISTRY}/nacos-server"
    docker push "${REGISTRY}/nacos-server"
    
    # Pull and retag Sentinel image
    docker pull bladex/sentinel-dashboard:1.8.6
    docker tag bladex/sentinel-dashboard:1.8.6 "${REGISTRY}/sentinel-dashboard"
    docker push "${REGISTRY}/sentinel-dashboard:1.8.6"
    
    # Pull and retag Kafka image
    docker pull bitnami/kafka:3.4.0
    docker tag bitnami/kafka:3.4.0 "${REGISTRY}/kafka"
    docker push "${REGISTRY}/kafka"
    
    # Pull and retag Zookeeper image (for Kafka)
    docker pull bitnami/zookeeper:3.8.1
    docker tag bitnami/zookeeper:3.8.1 "${REGISTRY}/zookeeper"
    docker push "${REGISTRY}/zookeeper"
    
    Write-Host "All dependency images pulled and pushed successfully!" -ForegroundColor Green
}

function Build-And-Push-All-Complete {
    Build-And-Push-All
    Build-And-Push-All-Canary
    Pull-Dependency-Images
    
    Write-Host "All project and dependency images built and pushed successfully!" -ForegroundColor Green
}

switch ($Command) {
    "help" { Show-Help }
    "build-all-images" { Build-All-Images }
    "deploy-chart" { Deploy-Chart }
    "deploy-chart-prod" { Deploy-Chart-Prod }
    "deploy-chart-canary" { Deploy-Chart-Canary }
    "build-and-push-all" { Build-And-Push-All }
    "build-all-images-canary" { Build-All-Images-Canary }
    "push-all-images-canary" { Push-All-Images-Canary }
    "build-and-push-all-canary" { Build-And-Push-All-Canary }
    "pull-dependency-images" { Pull-Dependency-Images }
    "build-and-push-all-complete" { Build-And-Push-All-Complete }
    default { 
        Write-Host "Unknown command: $Command" -ForegroundColor Red
        Show-Help
    }
}