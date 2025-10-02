import requests
import logging

logging.basicConfig(level=logging.DEBUG)


def test_https_connection(url):
    try:
        logging.info(f"测试HTTPS连接到: {url}")
        response = requests.get(url)
        logging.info(f"连接成功: {response.status_code}")
        return True
    except requests.exceptions.SSLError as e:
        logging.error(f"SSL错误: {str(e)}")
        logging.error("可能的原因:")
        logging.error("1. 服务器证书已过期")
        logging.error("2. 证书不匹配域名")
        logging.error("3. 证书不受信任")

        # 尝试禁用证书验证（仅用于调试！）
        try:
            logging.warning("尝试禁用SSL验证（不安全，仅用于调试）")
            response = requests.get(url, verify=False)
            logging.info(f"禁用SSL验证后连接成功: {response.status_code}")
            logging.info("确认是SSL证书问题，需要正确处理证书")
        except Exception as e2:
            logging.error(f"即使禁用SSL验证也失败: {str(e2)}")

        return False


# 测试HTTPS连接
test_https_connection('https://expired.badssl.com/')  # 过期证书网站
test_https_connection('https://www.google.com/')  # 正常网站