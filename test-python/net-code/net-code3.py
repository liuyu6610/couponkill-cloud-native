import socket
import threading
from queue import Queue

# 目标IP和端口范围
target = "127.0.0.1"
port_range = range(1, 1025)
queue = Queue()
open_ports = []

# 将端口添加到队列
for port in port_range:
    queue.put(port)

# 扫描函数
def port_scan(port):
    try:
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.settimeout(1)
        result = sock.connect_ex((target, port))
        if result == 0:
            open_ports.append(port)
            print(f"端口 {port}: 开放")
        sock.close()
    except:
        pass

# 使用多线程扫描
def worker():
    while not queue.empty():
        port = queue.get()
        port_scan(port)
        queue.task_done()

# 创建线程
thread_count = 100
for _ in range(thread_count):
    thread = threading.Thread(target=worker)
    thread.daemon = True
    thread.start()

queue.join()
print(f"扫描完成! 共发现 {len(open_ports)} 个开放端口")
print(f"开放端口: {open_ports}")