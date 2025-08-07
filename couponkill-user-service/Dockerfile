# couponkill-user-service/Dockerfile
FROM openjdk:21-jdk-slim

WORKDIR /app

COPY target/couponkill-user-service-1.0-SNAPSHOT.jar app.jar

# 挂载路径
VOLUME /app/logs
VOLUME /app/config

# 启动参数
ENTRYPOINT ["java", "-jar", "app.jar", "--spring.profiles.active=prod"]

EXPOSE 8081