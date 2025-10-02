import asyncio
import json
import websockets

# 存储所有连接的客户端
connected_clients = set()


async def chat_server(websocket):
    # 新客户端连接
    connected_clients.add(websocket)
    try:
        # 向所有客户端广播新用户加入消息
        if len(connected_clients) > 0:
            await broadcast({"type": "system", "message": "新用户加入聊天室!"})

        # 接收来自客户端的消息
        async for message in websocket:
            data = json.loads(message)
            # 广播消息给所有客户端
            await broadcast({"type": "message", "user": data.get("user", "匿名"), "message": data.get("message", "")})

    finally:
        # 客户端断开连接时
        connected_clients.remove(websocket)
        await broadcast({"type": "system", "message": "有用户离开聊天室!"})


# 广播消息给所有连接的客户端
async def broadcast(message):
    if connected_clients:
        # 将消息转换为JSON字符串
        message_str = json.dumps(message)
        # 发送给所有连接的客户端
        await asyncio.gather(
            *[client.send(message_str) for client in connected_clients]
        )


async def main():
    async with websockets.serve(chat_server, "localhost", 8765):
        print("聊天服务器已启动，监听端口 8765...")
        await asyncio.Future()  # 持续运行


if __name__ == "__main__":
    asyncio.run(main())