import multiprocessing
import requests
import time
from bs4 import BeautifulSoup


def fetch_url(url):
    """获取网页内容并提取标题"""
    try:
        response = requests.get(url, timeout=10)
        # 确保只返回简单的、可序列化的数据类型
        soup = BeautifulSoup(response.text, 'html.parser')
        title = soup.title.string.strip() if soup.title else "No title"
        # 显式删除soup和response对象以确保不会被意外包含在返回结果中
        del soup
        del response
        return {"url": url, "title": title, "status": 200}
    except Exception as e:
        return {"url": url, "error": str(e)}


def crawl_parallel(urls, num_processes=4):
    """并行爬取多个URL"""
    with multiprocessing.Pool(processes=num_processes) as pool:
        results = pool.map(fetch_url, urls)
    return results


if __name__ == "__main__":
    # 示例URL列表
    urls_to_crawl = [
        "https://www.python.org",
        "https://docs.python.org",
        "https://pypi.org",
        "https://www.djangoproject.com",
        "https://flask.palletsprojects.com",
        "https://pandas.pydata.org",
        "https://numpy.org",
        "https://matplotlib.org"
    ]

    start_time = time.time()
    results = crawl_parallel(urls_to_crawl)
    end_time = time.time()

    print(f"爬取 {len(urls_to_crawl)} 个URL用时: {end_time - start_time:.2f}秒")

    # 显示结果
    for result in results:
        if "error" in result:
            print(f"{result['url']} - 错误: {result['error']}")
        else:
            print(f"{result['url']} - {result['status']} - {result['title'][:50]}...")