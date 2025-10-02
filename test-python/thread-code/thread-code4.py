import asyncio
import aiohttp
import time


async def fetch_url(url):
    async with aiohttp.ClientSession() as session:
        async with session.get(url) as response:
            return await response.text()


async def main():
    urls = [
        "http://python.org",
        "http://pypi.org",
        "http://docs.python.org"
    ]

    start = time.time()

    tasks = [fetch_url(url) for url in urls]
    pages = await asyncio.gather(*tasks)

    for url, page in zip(urls, pages):
        print(f"{url}: 页面大小 {len(page)} 字节")

    print(f"总耗时: {time.time() - start:.2f} 秒")
