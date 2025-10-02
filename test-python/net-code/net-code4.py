import requests
from bs4 import BeautifulSoup


def scrape_python_blog():
    """
    从Python官方博客获取最新文章的标题和链接
    """
    url = "https://blog.python.org/"

    # 发送HTTP请求
    response = requests.get(url)

    # 检查请求是否成功
    if response.status_code == 200:
        # 创建Beautiful Soup对象，使用html.parser解析器
        soup = BeautifulSoup(response.text, 'html.parser')

        # 查找文章条目 - 根据实际网站结构调整选择器
        # 网站使用的是Blogger平台，文章在具有'post'类的div中
        posts = soup.find_all('div', class_='post')

        blog_data = []
        for post in posts:
            # 提取标题和链接
            title_element = post.find('h3', class_='post-title')
            if title_element and title_element.a:
                title = title_element.a.text.strip()
                link = title_element.a['href']
                
                # 提取日期
                date_element = post.find('span', class_='post-timestamp')
                date = date_element.text.strip() if date_element else "未知日期"
                
                blog_data.append({
                    'title': title,
                    'link': link,
                    'date': date
                })
            
        # 如果上面的方法没有找到文章，尝试另一种方法
        if not blog_data:
            # 查找所有带链接的h2和h3标签作为文章标题
            title_elements = soup.find_all(['h2', 'h3'])
            for element in title_elements:
                link_element = element.find('a')
                if link_element:
                    title = link_element.text.strip()
                    link = link_element.get('href', '')
                    
                    # 尝试找到发布日期
                    date_element = soup.find('span', class_='post-timestamp')
                    date = date_element.text.strip() if date_element else "未知日期"
                    
                    blog_data.append({
                        'title': title,
                        'link': link,
                        'date': date
                    })
                    # 为了避免重复，我们限制只处理前几个
                    if len(blog_data) >= 10:
                        break

        return blog_data
    else:
        print(f"请求失败，状态码: {response.status_code}")
        return []


# 执行爬虫函数
python_blog_articles = scrape_python_blog()

# 显示结果
print(f"找到 {len(python_blog_articles)} 篇文章:")
for i, article in enumerate(python_blog_articles[:5], 1):  # 只显示前5篇
    print(f"{i}. {article['title']} ({article['date']})")
    print(f"   链接: {article['link']}")
    print()