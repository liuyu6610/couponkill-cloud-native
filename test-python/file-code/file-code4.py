from pathlib import Path
import os

# 当前工作目录
current_dir = Path.cwd()
print(f"当前工作目录: {current_dir}")

# 创建文件路径
file_path = current_dir / "test.txt"

# 写入文件
file_path.write_text("Hello, pathlib!")

# 检查是否存在
print(f"文件存在吗? {file_path.exists()}")

# 读取文件
content = file_path.read_text()
print(f"文件内容: {content}")

# 获取文件信息
print(f"文件大小: {file_path.stat().st_size} 字节")

# 迭代目录中的文件
print("\n当前目录内容:")
for item in current_dir.iterdir():
    type_str = "目录" if item.is_dir() else "文件"
    print(f"{item.name} - {type_str}")

# 创建目录(如果不存在)
new_dir = current_dir / "new_folder"
new_dir.mkdir(exist_ok=True)
print(f"\n创建了新目录: {new_dir}")

# 删除文件
if file_path.exists():
    file_path.unlink()
    print(f"删除了文件: {file_path}")

# 删除目录
if new_dir.exists():
    new_dir.rmdir()
    print(f"删除了目录: {new_dir}")