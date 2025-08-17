# 构建阶段：使用Maven镜像
FROM maven:3.9.6-eclipse-temurin-21 AS builder

# 设置工作目录（项目根目录）
WORKDIR /app

# 1. 先复制父工程POM并安装（关键步骤：让Maven识别父工程）
COPY pom.xml .
RUN mvn -f pom.xml clean install -N -DskipTests

# 2. 复制common模块的POM和源码，安装到本地仓库
COPY couponkill-common/pom.xml couponkill-common/
COPY couponkill-common/src couponkill-common/src/
RUN mvn -f couponkill-common/pom.xml clean install -DskipTests

# 3. 复制-service模块的POM和源码，构建打包
COPY couponkill-coupon-service/pom.xml couponkill-coupon-service/
COPY couponkill-coupon-service/src couponkill-coupon-service/src/
RUN mvn -f couponkill-coupon-service/pom.xml clean package -DskipTests

# 运行阶段：使用轻量JRE镜像
FROM eclipse-temurin:21-jre-alpine
WORKDIR /app

# 从构建阶段复制打包好的jar包
COPY --from=builder /app/couponkill-coupon-service/target/*.jar app.jar

# 暴露端口
EXPOSE 8081

# 启动命令
ENTRYPOINT ["java", "-jar", "app.jar"]