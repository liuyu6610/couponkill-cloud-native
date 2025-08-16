# 构建阶段
FROM maven:3.8.5-openjdk-17 AS builder
WORKDIR /app
# 复制相关模块
COPY couponkill-order-service/pom.xml .
COPY couponkill-order-service/src ./src
COPY couponkill-common ../couponkill-common
COPY pom.xml ../pom.xml

# 打包
RUN mvn -f pom.xml clean package -DskipTests

# 运行阶段
FROM openjdk:17-jdk-slim
WORKDIR /app
COPY --from=builder /app/target/*.jar app.jar

# 暴露端口
EXPOSE 8083

# 启动命令
ENTRYPOINT ["java", "-jar", "app.jar"]