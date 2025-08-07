# couponkill-user-service/Dockerfile
FROM openjdk:17-jdk-slim

WORKDIR /app

COPY target/couponkill-user-service-1.0-SNAPSHOT.jar app.jar

# 暴露端口
EXPOSE 8081

# 挂载目录
VOLUME ["/app/logs", "/app/config"]

# 启动命令
ENTRYPOINT ["java", "-jar", "app.jar", "--spring.profiles.active=${SPRING_PROFILES_ACTIVE:dev}"]