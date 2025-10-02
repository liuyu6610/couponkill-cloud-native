import struct


def write_custom_binary(filename, numbers, text):
    """
    写入自定义二进制格式文件
    文件结构:
    - 4字节: 整数数量
    - 每个整数占4字节
    - 4字节: 字符串长度
    - N字节: UTF-8编码的字符串
    """
    with open(filename, 'wb') as file:
        # 写入整数数量
        file.write(struct.pack('i', len(numbers)))

        # 写入所有整数
        for num in numbers:
            file.write(struct.pack('i', num))

        # 编码并写入字符串
        encoded_text = text.encode('utf-8')
        file.write(struct.pack('i', len(encoded_text)))
        file.write(encoded_text)


def read_custom_binary(filename):
    """从自定义二进制文件中读取数据"""
    with open(filename, 'rb') as file:
        # 读取整数数量
        count = struct.unpack('i', file.read(4))[0]

        # 读取所有整数
        numbers = []
        for _ in range(count):
            num = struct.unpack('i', file.read(4))[0]
            numbers.append(num)

        # 读取字符串
        text_length = struct.unpack('i', file.read(4))[0]
        text = file.read(text_length).decode('utf-8')

        return numbers, text


# 写入示例数据
write_custom_binary('data.bin', [10, 20, 30, 40], "Hello Binary World!")

# 读取数据并验证
nums, txt = read_custom_binary('data.bin')
print(f"读取的数字: {nums}")
print(f"读取的文本: {txt}")