import xml.etree.ElementTree as ET


# 解析配置文件
def read_config(filename):
    try:
        tree = ET.parse(filename)
        return tree
    except Exception as e:
        print(f"读取配置文件时出错: {e}")
        return None


# 更新网站设置
def update_settings(tree, settings):
    if tree is None:
        return False

    root = tree.getroot()
    settings_elem = root.find('settings')

    if settings_elem is None:
        settings_elem = ET.SubElement(root, 'settings')

    for key, value in settings.items():
        setting = settings_elem.find(key)
        if setting is not None:
            setting.text = str(value)
        else:
            new_setting = ET.SubElement(settings_elem, key)
            new_setting.text = str(value)

    return True


# 保存配置
def save_config(tree, filename):
    try:
        tree.write(filename, encoding='utf-8', xml_declaration=True)
        return True
    except Exception as e:
        print(f"保存配置文件时出错: {e}")
        return False


# 示例用法
if __name__ == "__main__":
    # 示例配置文件
    example_xml = """<?xml version="1.0" encoding="UTF-8"?>
    <website>
        <settings>
            <theme>default</theme>
            <homepage>index.html</homepage>
            <cache_time>3600</cache_time>
        </settings>
        <pages>
            <page id="1" title="首页" />
            <page id="2" title="关于我们" />
        </pages>
    </website>
    """

    # 写入示例文件
    with open('website_config.xml', 'w', encoding='utf-8') as f:
        f.write(example_xml)

    # 读取配置
    config = read_config('website_config.xml')

    # 更新设置
    new_settings = {
        'theme': 'dark',
        'cache_time': '7200',
        'debug_mode': 'true'  # 新设置项
    }

    if update_settings(config, new_settings):
        save_config(config, 'website_config_updated.xml')
        print("配置已成功更新")

    # 显示更新后的配置
    updated_config = read_config('website_config_updated.xml')
    if updated_config:
        settings = updated_config.getroot().find('settings')
        print("\n更新后的设置:")
        for setting in settings:
            print(f"{setting.tag}: {setting.text}")