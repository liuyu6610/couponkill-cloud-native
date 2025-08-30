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

# 3. 复制user-service模块的POM和源码，构建打包
COPY couponkill-user-service/pom.xml couponkill-user-service/
COPY couponkill-user-service/src couponkill-user-service/src/
RUN mvn -f couponkill-user-service/pom.xml clean package -DskipTests

# 运行阶段：使用轻量JRE镜像
FROM eclipse-temurin:21-jre-alpine
WORKDIR /app

# 从构建阶段复制打包好的jar包
COPY --from=builder /app/couponkill-user-service/target/*.jar app.jar

# 暴露端口
EXPOSE 8083

# JVM优化参数 - 为高并发场景优化
ENV JAVA_OPTS="-Xms1g -Xmx2g -XX:+UseG1GC -XX:MaxGCPauseMillis=200 -XX:+UnlockExperimentalVMOptions -XX:+UseStringDeduplication -XX:+UseCompressedOops -XX:+OptimizeStringConcat -XX:+UseLargePages"
ENV THREAD_POOL_OPTS="-Dserver.tomcat.max-threads=200 -Dserver.tomcat.min-spare-threads=20 -Dserver.tomcat.accept-count=100 -Dserver.tomcat.max-connections=5000"

# 启动命令
ENTRYPOINT ["sh", "-c", "java $JAVA_OPTS $THREAD_POOL_OPTS -jar app.jar"]