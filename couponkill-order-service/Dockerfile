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

# 3. 复制order-service模块的POM和源码，构建打包
COPY couponkill-order-service/pom.xml couponkill-order-service/
COPY couponkill-order-service/src couponkill-order-service/src/
RUN mvn -f couponkill-order-service/pom.xml clean package -DskipTests

# 运行阶段：使用轻量JRE镜像
FROM eclipse-temurin:21-jre-alpine
WORKDIR /app

# 从构建阶段复制打包好的jar包
COPY --from=builder /app/couponkill-order-service/target/*.jar app.jar

# 暴露端口
EXPOSE 8082

# JVM优化参数 - 为高并发场景优化
ENV JAVA_OPTS="-Xms2g -Xmx4g -XX:+UseG1GC -XX:MaxGCPauseMillis=200 -XX:+UnlockExperimentalVMOptions -XX:+UseStringDeduplication -XX:+UseCompressedOops -XX:+OptimizeStringConcat -XX:+UseLargePages"
ENV THREAD_POOL_OPTS="-Dserver.tomcat.max-threads=400 -Dserver.tomcat.min-spare-threads=50 -Dserver.tomcat.accept-count=300 -Dserver.tomcat.max-connections=10000"

# 启动命令
ENTRYPOINT ["sh", "-c", "java $JAVA_OPTS $THREAD_POOL_OPTS -jar app.jar"]