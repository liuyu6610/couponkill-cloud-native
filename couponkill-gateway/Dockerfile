# Gateway?WebFlux?JVM ?? = JAVA_TOOL_OPTIONS?? Helm ???
FROM maven:3.9.9-eclipse-temurin-25 AS builder
WORKDIR /app
COPY couponkill-gateway/pom.xml .
COPY couponkill-gateway/src ./src
COPY pom.xml ../pom.xml
RUN mvn -f pom.xml clean package -DskipTests

FROM eclipse-temurin:25-jre-alpine
WORKDIR /app
COPY --from=builder /app/target/*.jar app.jar
EXPOSE 8080

ENV JAVA_TOOL_OPTIONS="-XX:+UseZGC -XX:+ZGenerational -XX:+UseCompactObjectHeaders -XX:MaxRAMPercentage=75.0"
ENV THREAD_POOL_OPTS="-Dspring.threads.virtual.enabled=true"

ENTRYPOINT ["sh", "-c", "java $THREAD_POOL_OPTS -jar app.jar"]
