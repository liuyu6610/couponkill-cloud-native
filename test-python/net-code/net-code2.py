from http.server import HTTPServer, BaseHTTPRequestHandler
import json


class AdvancedHTTPRequestHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == '/':
            # 返回主页
            self.send_response(200)
            self.send_header('Content-type', 'text/html')
            self.end_headers()
            self.wfile.write(b'<html><body><h1>Welcome to my server!</h1></body></html>')
        elif self.path == '/api/data':
            # 返回JSON数据
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            data = {'message': 'This is API data', 'status': 'success'}
            self.wfile.write(json.dumps(data).encode())
        else:
            # 处理404错误
            self.send_response(404)
            self.send_header('Content-type', 'text/html')
            self.end_headers()
            self.wfile.write(b'<html><body><h1>404 Not Found</h1></body></html>')

    def do_POST(self):
        if self.path == '/api/submit':
            # 获取请求内容长度
            content_length = int(self.headers['Content-Length'])
            # 读取请求体
            post_data = self.rfile.read(content_length)

            # 处理接收到的数据
            try:
                data = json.loads(post_data.decode('utf-8'))
                # 返回成功响应
                self.send_response(200)
                self.send_header('Content-type', 'application/json')
                self.end_headers()
                response = {'message': 'Data received successfully', 'data': data}
                self.wfile.write(json.dumps(response).encode())
            except json.JSONDecodeError:
                # 返回错误响应
                self.send_response(400)
                self.send_header('Content-type', 'application/json')
                self.end_headers()
                response = {'error': 'Invalid JSON data'}
                self.wfile.write(json.dumps(response).encode())
        else:
            # 处理404错误
            self.send_response(404)
            self.send_header('Content-type', 'text/html')
            self.end_headers()
            self.wfile.write(b'<html><body><h1>404 Not Found</h1></body></html>')


# 创建服务器
httpd = HTTPServer(('localhost', 8000), AdvancedHTTPRequestHandler)
print("高级HTTP服务器已启动 http://localhost:8000")

# 启动服务器
httpd.serve_forever()