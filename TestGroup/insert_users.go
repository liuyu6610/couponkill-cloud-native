package main

import (
	"TestGroup/config"
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	// 指定配置文件路径
	configPath := `D:\couponkill\config.json`

	// 加载配置
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Println("加载配置失败：", err)
		return
	}

	// 构建数据库连接字符串
	dataSourceName := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s",
		cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName)

	// 连接数据库
	db, err := sql.Open("mysql", dataSourceName)
	if err != nil {
		fmt.Println("连接数据库失败：", err)
		return
	}
	defer db.Close()

	// 测试数据库连接
	if err := db.Ping(); err != nil {
		fmt.Println("数据库连接测试失败：", err)
		return
	}

	fmt.Println("数据库连接成功")

	// 插入2000个新用户到数据库
	fmt.Println("正在插入2000个新用户到数据库...")
	if err := insertUsers(db); err != nil {
		fmt.Println("插入用户失败：", err)
		return
	}
	fmt.Println("用户数据插入完成")
}

// insertUsers 向数据库插入500个新用户
func insertUsers(db *sql.DB) error {
	// 开始事务
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer tx.Rollback()

	// 准备插入用户语句
	userStmt, err := tx.Prepare("INSERT INTO user (id, username, password, phone, email, status, create_time, update_time) VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())")
	if err != nil {
		return fmt.Errorf("准备用户插入语句失败: %w", err)
	}
	defer userStmt.Close()

	// 准备插入用户优惠券统计语句
	countStmt, err := tx.Prepare("INSERT INTO user_coupon_count (user_id, total_count, seckill_count, normal_count, expired_count, update_time, version) VALUES (?, 0, 0, 0, 0, NOW(), 0)")
	if err != nil {
		return fmt.Errorf("准备统计插入语句失败: %w", err)
	}
	defer countStmt.Close()

	// 插入2000个用户 (ID从1000到3000),由于之前我使用500先测试，所以自己更改
	for i := 1501; i <= 3000; i++ {
		username := fmt.Sprintf("testuser%d", i)
		password := "$2a$10$u4m3J/40CC99e7KJJEcgM.R4B0n03sgRHZGH9bSziGeX9EWyBEu2m" // 默认密码123456
		phone := fmt.Sprintf("1380013%04d", i)
		email := fmt.Sprintf("test%d@example.com", i)
		// 插入用户
		_, err = userStmt.Exec(i, username, password, phone, email, 1)
		if err != nil {
			return fmt.Errorf("插入用户%d失败: %w", i, err)
		}

		// 插入用户优惠券统计
		_, err = countStmt.Exec(i)
		if err != nil {
			return fmt.Errorf("插入用户%d的统计信息失败: %w", i, err)
		}
	}

	// 提交事务
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	return nil
}
