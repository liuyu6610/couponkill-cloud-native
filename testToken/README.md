# TestToken 工具

该工具用于创建测试用户、登录并获取JWT Token，然后将用户ID和Token保存到CSV文件中，供JMeter性能测试使用。

## 功能说明

1. 批量注册测试用户
2. 登录获取JWT Token
3. 将用户ID和对应的Token保存到CSV文件中

## 目录结构

```
testToken/
├── README.md
├── bulk_insert_users/
│   ├── go.mod
│   ├── go.sum
│   └── main.go
├── generate_tokens/
│   ├── go.mod
│   ├── go.sum
│   └── main.go
```

## 使用方法

### 1. 配置数据库连接和API地址

在 [bulk_insert_users/main.go](file://D:/couponkill/couponkill-cloud-native/testToken/bulk_insert_users/main.go) 文件中修改以下配置：

```go
const (
    // 数据库配置
    dbUser     = "master0"
    dbPassword = "Yu20040925@"
    dbHost0    = "rm-bp19518a44a083lmyio.mysql.rds.aliyuncs.com"
    dbHost1    = "rm-bp19518a44a083lmyio.mysql.rds.aliyuncs.com"
    dbPort     = 3306
    dbName0    = "user_db_0"
    dbName1    = "user_db_1"
)
```

在 [generate_tokens/main.go](file://D:/couponkill/couponkill-cloud-native/testToken/generate_tokens/main.go) 文件中修改API地址：

```go
const (
    apiBaseURL = "http://localhost"
)
```

### 2. 安装依赖

```bash
cd bulk_insert_users
go mod tidy

cd ../generate_tokens
go mod tidy
```

### 3. 批量插入测试用户

首先需要确保有足够的测试用户，可以使用以下命令批量插入用户：

```bash
cd bulk_insert_users
go run main.go
```

默认会插入100000个用户（ID从1001开始）

### 4. 生成用户Token

运行以下命令为用户生成Token：

```bash
cd generate_tokens
go run main.go
```

程序会为3000个用户（ID从1001开始）登录并获取Token，然后保存到 `user_tokens.csv` 文件中。

### 5. 输出文件

程序运行后会生成 `user_tokens.csv` 文件，格式如下：

```csv
userId,token
1001,eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
1002,eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
...
```

## 分库分表规则

根据Nacos配置，用户服务采用以下分库分表规则：

1. 数据库分片：
   - user-db-0: 存储ID为偶数的用户
   - user-db-1: 存储ID为奇数的用户
   - 分片算法: user-db-$->{id % 2}

2. 用户优惠券统计表同样按照用户ID分片：
   - user_coupon_count表也分布在user-db-0和user-db-1中
   - 分片算法: user-db-$->{user_id % 2}

## 批量插入用户数据

为了方便测试，提供了批量插入用户数据的脚本：

1. 使用Go脚本批量插入用户：
   ```bash
   cd bulk_insert_users
   go run main.go
   ```

2. 或者通过用户服务API批量生成：
   ```bash
   curl -X POST "http://localhost/api/v1/user/batch/generate?startId=1001&count=10000"
   ```

## 在JMeter中使用生成的Token

生成的 `user_tokens.csv` 文件可以直接用于JMeter测试计划。测试计划会为每个用户使用对应的Token进行认证，确保请求能够正确通过网关的认证检查。

## 注意事项

1. 确保用户服务API可以访问（默认地址为 http://localhost）
2. 程序会先尝试通过API注册用户，如果用户已存在则会跳过注册
3. 生成的CSV文件可以直接用于JMeter测试
4. 根据实际环境调整API地址和数据库连接信息
5. 批量插入用户时会根据分库规则自动分配到对应的数据库
6. 确保在运行测试前已生成了足够的用户和对应的Token
7. 两个Go程序现在位于独立的模块中，避免了结构体和其他定义的重复问题