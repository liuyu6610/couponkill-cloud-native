import asyncio
import aiohttp
import time


async def fetch_url(url, session):
    start = time.time()
    async with session.get(url) as response:
        result = await response.text()
        end = time.time()
        print(f"获取 {url} 完成，用时 {end - start:.2f}秒")
        return len(result)


async def main():
    urls = [
        "https://www.python.org",
        "https://docs.python.org",
        "https://pypi.org",
        "https://github.com",
        "https://stackoverflow.com"
    ]

    start = time.time()

    async with aiohttp.ClientSession() as session:
        tasks = [fetch_url(url, session) for url in urls]
        results = await asyncio.gather(*tasks)

        for url, size in zip(urls, results):
            print(f"{url}: 大小 {size} 字节")

    end = time.time()
    print(f"总共用时: {end - start:.2f}秒")


# 运行主协程
if __name__ == "__main__":
    asyncio.run(main())