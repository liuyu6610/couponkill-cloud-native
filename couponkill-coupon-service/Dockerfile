# couponkill-coupon-service/Dockerfile
FROM openjdk:21-jdk-slim

WORKDIR /app

COPY target/couponkill-coupon-service-1.0-SNAPSHOT.jar app.jar

VOLUME /app/logs
VOLUME /app/config

ENTRYPOINT ["java", "-jar", "app.jar", "--spring.profiles.active=prod"]

EXPOSE 8082