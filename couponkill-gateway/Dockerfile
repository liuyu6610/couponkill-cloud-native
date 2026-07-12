FROM maven:3.9.9-eclipse-temurin-25 AS builder
WORKDIR /app
#复制pom.xml和源�?
COPY couponkill-gateway/pom.xml .
COPY couponkill-gateway/src ./src
COPY pom.xml ../pom.xml

# 打包（跳过测试）
RUN mvn -f pom.xml clean package -DskipTests


FROM eclipse-temurin:25-jre-alpine
WORKDIR /app
COPY --from=builder /app/target/*.jar app.jar

# JVM优化参数 - 为高并发网关场景优化
ENV JAVA_OPTS="-Xms1g -Xmx2g -XX:+UseG1GC -XX:MaxGCPauseMillis=200 -XX:+UnlockExperimentalVMOptions -XX:+UseStringDeduplication -XX:+UseCompressedOops -XX:+OptimizeStringConcat -XX:+UseLargePages"
ENV THREAD_POOL_OPTS="-Dserver.tomcat.max-threads=500 -Dserver.tomcat.min-spare-threads=50 -Dserver.tomcat.accept-count=500 -Dserver.tomcat.max-connections=20000"

# 暴露端口
EXPOSE 8080

# 启动命令
ENTRYPOINT ["sh", "-c", "java $JAVA_OPTS $THREAD_POOL_OPTS -jar app.jar"]