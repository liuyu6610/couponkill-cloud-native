import threading
import requests
import time
from queue import Queue
from urllib.parse import urlparse


class WebCrawler:
    def __init__(self, urls, max_threads=5):
        self.urls = urls
        self.results = {}
        self.queue = Queue()
        self.lock = threading.Lock()
        self.semaphore = threading.Semaphore(max_threads)

        # 对同一域名的请求使用单独的锁进行限制（避免对同一网站发送过多请求）
        self.domain_locks = {}
        self.domain_lock = threading.Lock()

        # 将所有URL放入队列
        for url in urls:
            self.queue.put(url)

    def get_domain_lock(self, url):
        domain = urlparse(url).netloc
        with self.domain_lock:
            if domain not in self.domain_locks:
                self.domain_locks[domain] = threading.Semaphore(2)  # 每个域名最多2个并发请求
            return self.domain_locks[domain]

    def crawl_worker(self):
        while not self.queue.empty():
            try:
                url = self.queue.get(block=False)
            except:
                break

            domain_semaphore = self.get_domain_lock(url)

            with self.semaphore, domain_semaphore:
                try:
                    print(f"爬取 {url}")
                    start_time = time.time()
                    response = requests.get(url, timeout=10)
                    elapsed = time.time() - start_time

                    with self.lock:
                        self.results[url] = {
                            'status_code': response.status_code,
                            'content_length': len(response.content),
                            'time': elapsed
                        }
                except Exception as e:
                    with self.lock:
                        self.results[url] = {'error': str(e)}
                finally:
                    self.queue.task_done()

    def crawl(self):
        threads = []
        start_time = time.time()

        # 创建工作线程
        for _ in range(10):  # 创建10个工作线程
            thread = threading.Thread(target=self.crawl_worker)
            thread.daemon = True
            thread.start()
            threads.append(thread)

        # 等待所有线程完成
        for thread in threads:
            thread.join()

        total_time = time.time() - start_time
        print(f"爬取完成，总耗时: {total_time:.2f}秒")
        return self.results


# 使用爬虫
urls = [
    "https://www.python.org",
    "https://www.github.com",
    "https://www.stackoverflow.com",
    "https://www.wikipedia.org",
    "https://www.reddit.com",
    "https://www.google.com",
    "https://www.youtube.com",
    "https://www.microsoft.com"
]

crawler = WebCrawler(urls)
results = crawler.crawl()

# 显示结果
print("\n爬取结果:")
for url, result in results.items():
    if 'error' in result:
        print(f"{url} - 错误: {result['error']}")
    else:
        print(
            f"{url} - 状态码: {result['status_code']}, 内容大小: {result['content_length'] / 1024:.2f} KB, 耗时: {result['time']:.2f}s")