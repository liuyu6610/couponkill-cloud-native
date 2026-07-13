# ?????Maven + JDK25
FROM maven:3.9.9-eclipse-temurin-25 AS builder
WORKDIR /app

COPY pom.xml .
RUN mvn -f pom.xml clean install -N -DskipTests

COPY couponkill-common/pom.xml couponkill-common/
COPY couponkill-common/src couponkill-common/src/
RUN mvn -f couponkill-common/pom.xml clean install -DskipTests

COPY couponkill-user-service/pom.xml couponkill-user-service/
COPY couponkill-user-service/src couponkill-user-service/src/
RUN mvn -f couponkill-user-service/pom.xml clean package -DskipTests

FROM eclipse-temurin:25-jre-alpine
WORKDIR /app
COPY --from=builder /app/couponkill-user-service/target/*.jar app.jar
EXPOSE 8083

ENV JAVA_TOOL_OPTIONS="-XX:+UseZGC -XX:+ZGenerational -XX:+UseCompactObjectHeaders -XX:MaxRAMPercentage=75.0"
ENV THREAD_POOL_OPTS="-Dspring.threads.virtual.enabled=true"

ENTRYPOINT ["sh", "-c", "java $THREAD_POOL_OPTS -jar app.jar"]
