import struct
import wave


def print_wav_info(wav_file):
    """打印WAV文件的基本信息"""
    with wave.open(wav_file, 'rb') as wav:
        # 获取基本信息
        n_channels = wav.getnchannels()
        sample_width = wav.getsampwidth()
        frame_rate = wav.getframerate()
        n_frames = wav.getnframes()
        compression_type = wav.getcomptype()
        compression_name = wav.getcompname()

        # 计算时长（秒）
        duration = n_frames / frame_rate

        # 打印信息
        print(f"文件: {wav_file}")
        print(f"通道数: {n_channels}")
        print(f"采样宽度: {sample_width} 字节")
        print(f"采样率: {frame_rate} Hz")
        print(f"帧数: {n_frames}")
        print(f"压缩类型: {compression_type}")
        print(f"压缩名称: {compression_name}")
        print(f"时长: {duration:.2f} 秒")


# 使用示例
try:
    print_wav_info('example.wav')  # 需要有一个WAV文件用于测试
except FileNotFoundError:
    print("请替换为有效的WAV文件路径")