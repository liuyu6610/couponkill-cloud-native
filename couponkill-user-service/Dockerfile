# 构建阶段
FROM maven:3.8.5-openjdk-17 AS builder
WORKDIR /app
# 复制pom.xml和源码
COPY couponkill-user-service/pom.xml .
COPY couponkill-user-service/src ./src
COPY couponkill-common ../couponkill-common
COPY pom.xml ../pom.xml

# 打包（跳过测试）
RUN mvn -f pom.xml clean package -DskipTests

# 运行阶段
FROM openjdk:17-jdk-slim
WORKDIR /app
# 从构建阶段复制jar包
COPY --from=builder /app/target/*.jar app.jar

# 暴露端口（与bootstrap.yml一致）
EXPOSE 8082

# 启动命令（使用默认配置，依赖内置的bootstrap.yml）
ENTRYPOINT ["java", "-jar", "app.jar"]