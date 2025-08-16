FROM maven:3.8.5-openjdk-17 AS builder
WORKDIR /app
//复制pom.xml和源码
COPY couponkill-gateway/pom.xml .
COPY couponkill-gateway/src ./src
COPY pom.xml ../pom.xml

# 打包（跳过测试）
RUN mvn -f pom.xml clean package -DskipTests


From openjdk:17-slim
WORKDIR /app
COPY --from=builder /app/target/*.jar app.jar
EXPOSE 8080
CMD ["java", "-jar", "app.jar"]
