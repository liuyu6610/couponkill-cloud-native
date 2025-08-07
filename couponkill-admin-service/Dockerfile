FROM openjdk:21-jdk-slim
WORKDIR /app
COPY target/spring-boot-docker-0.0.1-SNAPSHOT.jar app.jar
# 挂载路径
VOLUME /app/logs
VOLUME /app/config

ENTRYPOINT ["java","-jar","app.jar"]
EXPOSE 8080