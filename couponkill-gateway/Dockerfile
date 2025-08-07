# couponkill-gateway/Dockerfile
FROM openjdk:21-jdk-slim

WORKDIR /app

COPY target/couponkill-gateway-1.0-SNAPSHOT.jar app.jar

# 暴露端口
EXPOSE 8080

# JVM参数优化
ENTRYPOINT ["java", "-jar", "-Xms512m", "-Xmx512m", "app.jar"]