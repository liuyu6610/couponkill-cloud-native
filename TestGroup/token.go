package main

import (
	"TestGroup/config"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url" // 正确导入 net/url 包
	"os"
	"path/filepath"
	"time"
)

// LoginResponse 代表登录接口的响应结构
type LoginResponse struct {
	Code int                    `json:"code"`
	Data map[string]interface{} `json:"data"`
	Msg  string                 `json:"message"`
}

func main() {
	// 指定配置文件路径
	configPath := `D:\couponkill\config.json`

	// 加载配置
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Println("加载配置失败：", err)
		return
	}

	// 获取用户主目录
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("获取用户主目录失败：", err)
		return
	}

	// 构建文件路径
	desktop := filepath.Join(homeDir, "Desktop")
	filePath := filepath.Join(desktop, "tokens.csv")

	// 创建文件
	file, err := os.Create(filePath)
	if err != nil {
		fmt.Println("创建文件失败：", err)
		return
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Println("关闭文件失败：", err)
		}
	}()

	// 创建缓冲写入器
	writer := bufio.NewWriter(file)
	defer func() {
		if err := writer.Flush(); err != nil {
			fmt.Println("刷新缓冲区失败：", err)
		}
	}()

	// 创建HTTP客户端，设置超时
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// 使用网关地址
	baseURL := cfg.GatewayURL
	loginURL := fmt.Sprintf("%s/api/v1/user/login", baseURL)
	fmt.Printf("正在连接到: %s\n", loginURL)

	// 测试连接
	_, _, err = loginAndGetToken(client, loginURL, "liuyu", "123456")
	if err != nil {
		fmt.Printf("无法连接到服务: %v\n", err)
		fmt.Println("请确保服务已在运行")
		return
	}

	fmt.Println("连接成功，开始批量生成Token...")

	// 生成1001-1500的Token
	successCount := 0
	failedCount := 0

	for userId := 1001; userId <= 1500; userId++ {
		username := fmt.Sprintf("testuser%d", userId)
		// 使用统一密码
		password := "123456"

		token, userID, err := loginAndGetToken(client, loginURL, username, password)
		if err != nil {
			fmt.Printf("生成用户%s的Token失败: %v\n", username, err)
			failedCount++
			continue
		}

		// 写入Token到文件 (CSV格式，包含userId和token两列)
		if _, err := fmt.Fprintf(writer, "%d,%s\n", int(userID), token); err != nil {
			fmt.Printf("写入用户%s的Token失败: %v\n", username, err)
			return
		}

		successCount++

		// 每50个Token刷新一次并打印进度
		if userId%50 == 0 {
			if err := writer.Flush(); err != nil {
				fmt.Printf("刷新缓冲区失败: %v\n", err)
				return
			}
			fmt.Printf("已处理%d个用户 (成功:%d, 失败:%d)\n", userId-1000, successCount, failedCount)
		}
	}

	fmt.Printf("tokens_for_jmeter.csv生成完成，路径：%s，成功生成%d个Token，失败%d个\n", filePath, successCount, failedCount)
}

// loginAndGetToken 通过登录获取Token
// 注意：这里将 url 参数重命名为 targetURL 以避免与 net/url 包名冲突
func loginAndGetToken(client *http.Client, targetURL, username, password string) (string, float64, error) {
	// 构造表单数据
	formData := make(url.Values) // 使用 url.Values 创建表单数据
	formData.Set("username", username)
	formData.Set("password", password)

	// 创建登录请求
	req, err := http.NewRequest("POST", targetURL, bytes.NewBufferString(formData.Encode()))
	if err != nil {
		return "", 0, fmt.Errorf("构造登录请求失败: %w", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("发送登录请求失败: %w", err)
	}
	defer func() {
		if resp.Body != nil {
			resp.Body.Close()
		}
	}()

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("HTTP请求失败，状态码: %d", resp.StatusCode)
	}

	// 解析响应
	var loginResp LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return "", 0, fmt.Errorf("解析响应失败: %w", err)
	}

	// 检查业务状态码
	if loginResp.Code != 0 { // 0表示成功
		return "", 0, fmt.Errorf("登录失败，业务码: %d, 信息: %s", loginResp.Code, loginResp.Msg)
	}

	// 获取Token和用户ID
	token, ok := loginResp.Data["token"].(string)
	if !ok {
		return "", 0, fmt.Errorf("响应中未找到token")
	}

	userID, ok := loginResp.Data["userId"].(float64)
	if !ok {
		return "", 0, fmt.Errorf("响应中未找到userId")
	}

	return token, userID, nil
}
