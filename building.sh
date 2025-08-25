echo Building couponkill-user-service...
docker build -f couponkill-user-service/Dockerfile -t couponkill-user-service .

echo Building couponkill-coupon-service...
docker build -f couponkill-coupon-service/Dockerfile -t couponkill-coupon-service .

echo Building couponkill-order-service...
docker build -f couponkill-order-service/Dockerfile -t couponkill-order-service .

echo Building couponkill-gateway...
docker build -f couponkill-gateway/Dockerfile -t couponkill-gateway .

echo Building couponkill-go-service...
docker build -f couponkill-go-service/Dockerfile -t couponkill-go-service .

echo Building operator-coupon...
docker build -f operator-coupon/Dockerfile -t operator-coupon .


#Linux/Mac shell!/bin/bash

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

echo "All images built successfully!"
