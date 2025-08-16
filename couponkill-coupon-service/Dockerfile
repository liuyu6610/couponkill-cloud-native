# 构建阶段
FROM maven:3.8.5-openjdk-17 AS builder
WORKDIR /app
# 复制依赖模块和当前模块代码
COPY couponkill-coupon-service/pom.xml .
COPY couponkill-coupon-service/src ./src
COPY couponkill-common ../couponkill-common
COPY pom.xml ../pom.xml

# 打包
RUN mvn -f pom.xml clean package -DskipTests

# 运行阶段
FROM openjdk:17-jdk-slim
WORKDIR /app
COPY --from=builder /app/target/*.jar app.jar

# 暴露端口（与配置一致）
EXPOSE 8081

# 启动命令（依赖内置bootstrap.yml读取nacos配置）
ENTRYPOINT ["java", "-jar", "app.jar"]