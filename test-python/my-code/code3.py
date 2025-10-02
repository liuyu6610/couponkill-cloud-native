import os
import json
import csv
from datetime import datetime


class FileHandler:
    @staticmethod
    def get_extension(filename):
        """获取文件扩展名"""
        return os.path.splitext(filename)[1].lower()

    @staticmethod
    def read_text(filepath):
        """读取文本文件"""
        with open(filepath, 'r', encoding='utf-8') as f:
            return f.read()

    @staticmethod
    def write_text(filepath, content):
        """写入文本文件"""
        with open(filepath, 'w', encoding='utf-8') as f:
            f.write(content)

    @staticmethod
    def read_json(filepath):
        """读取JSON文件"""
        with open(filepath, 'r', encoding='utf-8') as f:
            return json.load(f)

    @staticmethod
    def write_json(filepath, data):
        """写入JSON文件"""
        with open(filepath, 'w', encoding='utf-8') as f:
            json.dump(data, f, ensure_ascii=False, indent=2)

    @staticmethod
    def read_csv(filepath):
        """读取CSV文件"""
        data = []
        with open(filepath, 'r', encoding='utf-8') as f:
            reader = csv.reader(f)
            for row in reader:
                data.append(row)
        return data

    @staticmethod
    def write_csv(filepath, data):
        """写入CSV文件"""
        with open(filepath, 'w', encoding='utf-8', newline='') as f:
            writer = csv.writer(f)
            writer.writerows(data)

    @staticmethod
    def backup_file(filepath):
        """创建文件备份"""
        if not os.path.exists(filepath):
            raise FileNotFoundError(f"无法备份不存在的文件: {filepath}")

        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        filename, extension = os.path.splitext(filepath)
        backup_path = f"{filename}_backup_{timestamp}{extension}"

        with open(filepath, 'rb') as src, open(backup_path, 'wb') as dst:
            dst.write(src.read())

        return backup_path


# 使用FileHandler类
try:
    # 写入文本文件
    FileHandler.write_text("example.txt", "这是一个示例文本文件")

    # 读取文本文件
    content = FileHandler.read_text("example.txt")
    print(f"文本内容: {content}")

    # 写入JSON数据
    data = {"name": "小明", "age": 18, "scores": [85, 92, 78]}
    FileHandler.write_json("student.json", data)

    # 读取JSON数据
    student = FileHandler.read_json("student.json")
    print(f"学生信息: {student}")

    # 备份文件
    backup_path = FileHandler.backup_file("example.txt")
    print(f"文件已备份到: {backup_path}")

except Exception as e:
    print(f"发生错误: {e}")