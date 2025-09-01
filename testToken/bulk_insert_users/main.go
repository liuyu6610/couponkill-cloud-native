package main

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	_ "github.com/go-sql-driver/mysql"
)

const (
	// 数据库配置
	dbUser     = "master0"
	dbPassword = "Yu20040925@"
	dbHost0    = "rm-bp19518a44a083lmyio.mysql.rds.aliyuncs.com"
	dbHost1    = "rm-bp19518a44a083lmyio.mysql.rds.aliyuncs.com"
	dbPort     = 3306
	dbName0    = "user_db_0"
	dbName1    = "user_db_1"

	// 批量插入配置 - 减小批次大小以减少锁竞争
	batchSize  = 100 // 从1000减小到100
	numWorkers = 5   // 从10减小到5
	totalUsers = 100000
)

type User struct {
	ID       int64
	Username string
	Password string
	Phone    string
	Email    string
}

func main() {
	fmt.Printf("开始批量插入 %d 个用户到分库分表中\n", totalUsers)

	startTime := time.Now()

	// 创建数据库连接池
	db0, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", dbUser, dbPassword, dbHost0, dbPort, dbName0))
	if err != nil {
		log.Fatal("连接数据库 user_db_0 失败:", err)
	}
	defer db0.Close()

	db1, err := sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", dbUser, dbPassword, dbHost1, dbPort, dbName1))
	if err != nil {
		log.Fatal("连接数据库 user_db_1 失败:", err)
	}
	defer db1.Close()

	// 测试连接
	if err := db0.Ping(); err != nil {
		log.Fatal("Ping 数据库 user_db_0 失败:", err)
	}

	if err := db1.Ping(); err != nil {
		log.Fatal("Ping 数据库 user_db_1 失败:", err)
	}

	fmt.Println("数据库连接成功")

	// 批量插入用户
	insertUsers(db0, db1)

	elapsed := time.Since(startTime)
	fmt.Printf("批量插入完成，耗时: %s\n", elapsed)
}

// generatePasswordHash 生成密码的BCrypt哈希
func generatePasswordHash(password string) string {
	// 使用成本因子10确保与Java BCryptPasswordEncoder生成的一致
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		log.Fatal("生成密码哈希失败:", err)
	}
	return string(hashedPassword)
}

func insertUsers(db0, db1 *sql.DB) {
	// 使用工作池模式并发插入
	userChan := make(chan User, batchSize)
	var wg sync.WaitGroup

	// 启动工作协程
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go worker(i, db0, db1, userChan, &wg)
	}

	// 生成用户数据
	go func() {
		defer close(userChan)
		for i := int64(1); i <= int64(totalUsers); i++ {
			user := User{
				ID:       i + 1000, // 从1001开始
				Username: fmt.Sprintf("testuser%d", i+1000),
				Password: generatePasswordHash("123456"),
				Phone:    fmt.Sprintf("138%08d", (i+1000)%100000000),
				Email:    fmt.Sprintf("testuser%d@example.com", i+1000),
			}
			userChan <- user
		}
	}()

	// 等待所有工作完成
	wg.Wait()
	fmt.Println("所有用户插入完成")
}

func worker(id int, db0, db1 *sql.DB, users <-chan User, wg *sync.WaitGroup) {
	defer wg.Done()

	count := 0
	batch0 := make([]User, 0, batchSize)
	batch1 := make([]User, 0, batchSize)

	for user := range users {
		// 根据分库规则分配用户
		if user.ID%2 == 0 {
			batch0 = append(batch0, user)
		} else {
			batch1 = append(batch1, user)
		}

		count++

		// 当批次达到设定大小时执行插入
		if len(batch0) >= batchSize {
			insertBatch(db0, batch0, 0)
			batch0 = batch0[:0] // 清空批次
		}

		if len(batch1) >= batchSize {
			insertBatch(db1, batch1, 1)
			batch1 = batch1[:0] // 清空批次
		}
	}

	// 插入剩余数据
	if len(batch0) > 0 {
		insertBatch(db0, batch0, 0)
	}

	if len(batch1) > 0 {
		insertBatch(db1, batch1, 1)
	}

	fmt.Printf("Worker %d 完成，处理了 %d 个用户\n", id, count)
}

func insertBatch(db *sql.DB, users []User, dbIndex int) {
	if len(users) == 0 {
		return
	}

	// 构建批量插入语句 - 使用明确的表前缀
	valueStrings := make([]string, 0, len(users))
	valueArgs := make([]interface{}, 0, len(users)*8)

	for _, user := range users {
		valueStrings = append(valueStrings, "(?, ?, ?, ?, ?, ?, NOW(), NOW())")
		valueArgs = append(valueArgs, user.ID, user.Username, user.Password, user.Phone, user.Email, 1)
	}

	// 使用明确的表名格式
	stmt := fmt.Sprintf("INSERT IGNORE INTO `user` (id, username, password, phone, email, status, create_time, update_time) VALUES %s",
		strings.Join(valueStrings, ","))

	// 执行批量插入
	_, err := db.Exec(stmt, valueArgs...)
	if err != nil {
		log.Printf("数据库 user_db_%d 批量插入用户失败: %v", dbIndex, err)
		return
	}

	// 为这些用户初始化优惠券统计记录
	valueStrings2 := make([]string, 0, len(users))
	valueArgs2 := make([]interface{}, 0, len(users)*7)

	for _, user := range users {
		valueStrings2 = append(valueStrings2, "(?, ?, ?, ?, ?, NOW(), ?)")
		valueArgs2 = append(valueArgs2, user.ID, 0, 0, 0, 0, 0)
	}

	stmt2 := fmt.Sprintf("INSERT IGNORE INTO `user_coupon_count` (user_id, total_count, seckill_count, normal_count, expired_count, update_time, version) VALUES %s",
		strings.Join(valueStrings2, ","))

	_, err = db.Exec(stmt2, valueArgs2...)
	if err != nil {
		log.Printf("数据库 user_db_%d 批量插入用户优惠券统计失败: %v", dbIndex, err)
		return
	}

	fmt.Printf("数据库 user_db_%d 成功插入 %d 个用户\n", dbIndex, len(users))
}

// 添加辅助函数用于连接字符串
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}

	// 使用strings.Join提高性能
	return strings.Join(strs, sep)
}
