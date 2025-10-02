import requests
from bs4 import BeautifulSoup
import re


def crawl_webpage(url):
    # 发送GET请求
    headers = {'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36'}
    response = requests.get(url, headers=headers)

    # 检查是否成功获取页面
    if response.status_code != 200:
        print(f"无法访问页面，状态码: {response.status_code}")
        return

    # 使用BeautifulSoup解析HTML
    soup = BeautifulSoup(response.text, 'html.parser')

    # 提取所有链接
    links = soup.find_all('a', href=True)

    # 打印页面标题和链接
    print(f"页面标题: {soup.title.string}")
    print(f"找到 {len(links)} 个链接:")

    for i, link in enumerate(links[:10], 1):  # 只显示前10个链接
        href = link['href']
        # 如果链接是相对路径，转换为绝对URL
        if not href.startswith('http'):
            href = re.sub(r'^/', '', href)  # 移除开头的斜杠
            href = f"{'/'.join(url.split('/')[:3])}/{href}"  # 构建完整URL
        print(f"{i}. {link.text.strip()[:30]}... -> {href}")


# 使用示例
if __name__ == "__main__":
    crawl_webpage("https://www.python.org")