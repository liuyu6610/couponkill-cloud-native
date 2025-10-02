import asyncio
import json
import websockets
import random


async def chat_client():
    # 为测试随机生成用户名
    user_id = f"用户{random.randint(1000, 9999)}"

    # 连接到服务器
    async with websockets.connect("ws://localhost:8765") as websocket:
        print(f"已连接到聊天服务器，你的用户名是: {user_id}")

        # 创建两个任务：一个发送消息，一个接收消息
        send_task = asyncio.create_task(send_messages(websocket, user_id))
        receive_task = asyncio.create_task(receive_messages(websocket))

        # 等待任一任务完成
        await asyncio.gather(send_task, receive_task)


# 发送消息的协程
async def send_messages(websocket, user_id):
    try:
        while True:
            message = input("输入消息 (或输入'exit'退出): ")
            if message.lower() == "exit":
                break

            # 构造消息并发送
            await websocket.send(json.dumps({
                "user": user_id,
                "message": message
            }))
    except Exception as e:
        print(f"发送消息出错: {e}")


# 接收消息的协程
async def receive_messages(websocket):
    try:
        async for message in websocket:
            data = json.loads(message)

            if data["type"] == "system":
                print(f"系统: {data['message']}")
            else:
                print(f"{data['user']}: {data['message']}")
    except Exception as e:
        print(f"接收消息出错: {e}")


if __name__ == "__main__":
    asyncio.run(chat_client())