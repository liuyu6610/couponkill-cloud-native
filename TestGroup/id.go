package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	// 获取用户主目录
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("获取用户主目录失败：", err)
		return
	}

	// 构建桌面路径和文件路径
	desktop := filepath.Join(homeDir, "Desktop")
	filePath := filepath.Join(desktop, "user_ids.csv")

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
	// 写入CSV头部
	if _, err := fmt.Fprintln(writer, "userId"); err != nil {
		fmt.Println("写入头部失败：", err)
		return
	}
	// 写入用户ID
	for i := 1001; i <= 1500; i++ {
		if _, err := fmt.Fprintf(writer, "%d\n", i); err != nil {
			fmt.Println("写入失败：", err)
			return
		}
	}

	fmt.Printf("user_ids.csv生成成功，路径：%s\n", filePath)
}
