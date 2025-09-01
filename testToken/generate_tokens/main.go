package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	apiBaseURL = "http://localhost"
	totalUsers = 3000
	startID    = 1001
)

type UserToken struct {
	UserID int64
	Token  string
}

// API响应格式
type LoginResponse struct {
	TypeData []interface{} `json:"-"`
	Code     int           `json:"code"`
	Message  string        `json:"message"`
	Data     LoginData     `json:"data"`
}

type LoginData struct {
	Token    string `json:"token"`
	UserID   int64  `json:"userId"`
	Username string `json:"username"`
}

type User struct {
	ID       int64
	Username string
	Password string
	Phone    string
	Email    string
}

func main() {
	fmt.Printf("开始为 %d 个用户生成token\n", totalUsers)

	// 创建用户列表
	var users []User
	for i := 0; i < totalUsers; i++ {
		id := startID + i
		user := User{
			ID:       int64(id),
			Username: fmt.Sprintf("testuser%d", id),
			Password: "123456", // 使用明文密码登录，与插入时一致
			Phone:    fmt.Sprintf("1380000%04d", id%10000),
			Email:    fmt.Sprintf("testuser%d@example.com", id),
		}
		users = append(users, user)
	}

	// 获取用户token
	userTokens := make([]UserToken, 0, len(users))
	client := &http.Client{Timeout: 10 * time.Second}

	for _, user := range users {
		fmt.Printf("正在处理用户 %s...\n", user.Username)

		// 登录获取token
		token, err := loginUser(user, client)
		if err != nil {
			log.Printf("用户 %s 登录失败: %v", user.Username, err)
			continue
		}

		userTokens = append(userTokens, UserToken{
			UserID: user.ID,
			Token:  token,
		})
	}

	// 保存到CSV文件
	if err := saveToCSV(userTokens); err != nil {
		log.Fatal("保存到CSV文件失败:", err)
	}

	fmt.Printf("完成，共生成 %d 个用户token\n", len(userTokens))
	fmt.Println("用户token已保存到 user_tokens.csv 文件中")
}

func loginUser(user User, client *http.Client) (string, error) {
	// 构造登录请求
	loginURL := fmt.Sprintf("%s/api/v1/user/login", apiBaseURL)

	// 使用表单格式
	formData := url.Values{}
	formData.Set("username", user.Username)
	formData.Set("password", user.Password)

	req, err := http.NewRequest("POST", loginURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return "", fmt.Errorf("创建登录请求失败: %v", err)
	}

	// 设置表单内容类型
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("发送登录请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取登录响应失败: %v", err)
	}

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("登录请求返回状态码 %d: %s", resp.StatusCode, string(body))
	}

	// 解析token
	var loginResp LoginResponse
	if err := json.Unmarshal(body, &loginResp); err != nil {
		return "", fmt.Errorf("解析登录响应失败: %v, 响应内容: %s", err, string(body))
	}

	if loginResp.Code != 0 {
		return "", fmt.Errorf("登录失败: %s", loginResp.Message)
	}

	return loginResp.Data.Token, nil
}

func saveToCSV(userTokens []UserToken) error {
	// 创建CSV文件
	file, err := os.Create("user_tokens.csv")
	if err != nil {
		return fmt.Errorf("创建CSV文件失败: %v", err)
	}
	defer file.Close()

	// 写入数据
	_, err = file.WriteString("userId,token\n")
	if err != nil {
		return fmt.Errorf("写入CSV头部失败: %v", err)
	}

	// 写入数据
	for _, ut := range userTokens {
		line := fmt.Sprintf("%d,%s\n", ut.UserID, ut.Token)
		_, err := file.WriteString(line)
		if err != nil {
			return fmt.Errorf("写入CSV记录失败: %v", err)
		}
	}

	return nil
}
