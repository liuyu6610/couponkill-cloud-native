import socket
import logging

logging.basicConfig(level=logging.DEBUG)

def check_dns(hostname):
    try:
        logging.debug(f"尝试解析主机名: {hostname}")
        ip_address = socket.gethostbyname(hostname)
        logging.info(f"解析成功: {hostname} -> {ip_address}")
        return ip_address
    except socket.gaierror:
        logging.error(f"无法解析主机名: {hostname}")
        logging.error("可能的原因: DNS服务器不可用或主机名不存在")
        return None

# 测试DNS解析
check_dns('www.google.com')
check_dns('nonexistent-domain-123456.com')