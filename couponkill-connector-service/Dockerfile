# Connector：不在秒杀热路径；JVM 与其它 Java 服务对齐 JAVA_TOOL_OPTIONS
FROM maven:3.9.9-eclipse-temurin-25 AS builder
WORKDIR /app

COPY pom.xml .
RUN mvn -f pom.xml clean install -N -DskipTests

COPY couponkill-common/pom.xml couponkill-common/
COPY couponkill-common/src couponkill-common/src/
RUN mvn -f couponkill-common/pom.xml clean install -DskipTests

COPY couponkill-connector-service/pom.xml couponkill-connector-service/
COPY couponkill-connector-service/src couponkill-connector-service/src/
RUN mvn -f couponkill-connector-service/pom.xml clean package -DskipTests

FROM eclipse-temurin:25-jre-alpine
WORKDIR /app
COPY --from=builder /app/couponkill-connector-service/target/*.jar app.jar
EXPOSE 8085

ENV JAVA_TOOL_OPTIONS="-XX:+UseZGC -XX:+ZGenerational -XX:+UseCompactObjectHeaders -XX:MaxRAMPercentage=75.0"
ENV THREAD_POOL_OPTS="-Dspring.threads.virtual.enabled=true"

ENTRYPOINT ["sh", "-c", "java $THREAD_POOL_OPTS -jar app.jar"]
