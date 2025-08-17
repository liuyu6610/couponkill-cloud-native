FROM maven:3.9.6-eclipse-temurin-21 AS builder
WORKDIR /app
#复制pom.xml和源码
COPY couponkill-gateway/pom.xml .
COPY couponkill-gateway/src ./src
COPY pom.xml ../pom.xml

# 打包（跳过测试）
RUN mvn -f pom.xml clean package -DskipTests


From eclipse-temurin:21-jre-alpine
WORKDIR /app
COPY --from=builder /app/target/*.jar app.jar
EXPOSE 8080
CMD ["java", "-jar", "app.jar"]
