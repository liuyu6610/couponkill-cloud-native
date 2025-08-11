在各服务 pom.xml 中加入：

```xml
<dependency>
  <groupId>org.springdoc</groupId>
  <artifactId>springdoc-openapi-starter-webmvc-ui</artifactId>
  <version>2.5.0</version>
</dependency>
<dependency>
  <groupId>com.github.xiaoymin</groupId>
  <artifactId>knife4j-openapi3-jakarta-spring-boot-starter</artifactId>
  <version>4.5.0</version>
</dependency>
```

启动后访问：
- Swagger UI: `http://<host>:<port>/swagger-ui/index.html`
- Knife4j: `http://<host>:<port>/doc.html`
