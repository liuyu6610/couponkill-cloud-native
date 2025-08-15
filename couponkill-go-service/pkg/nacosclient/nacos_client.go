package nacosclient

import (
	"fmt"
	"log"
	"net"
	"strconv"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/config_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

type NacosClient struct {
	client config_client.IConfigClient
}

func NewNacosClient(serverAddr, namespaceId string) (*NacosClient, error) {
	// 创建客户端配置
	clientConfig := constant.ClientConfig{
		NamespaceId:         namespaceId, // 命名空间ID
		TimeoutMs:           5000,
		NotLoadCacheAtStart: true,
		LogDir:              "/tmp/nacos/log",
		CacheDir:            "/tmp/nacos/cache",
		LogLevel:            "debug",
	}

	host, portStr, err := net.SplitHostPort(serverAddr)
	if err != nil {
		return nil, fmt.Errorf("解析Nacos服务器地址失败: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("解析Nacos端口失败: %v", err)
	}

	serverConfigs := []constant.ServerConfig{
		{
			IpAddr:      host, // 仅IP地址
			ContextPath: "/nacos",
			Port:        uint64(port), // 正确解析的端口
			Scheme:      "http",
		},
	}

	// 创建配置客户端
	configClient, err := clients.CreateConfigClient(map[string]interface{}{
		"serverConfigs": serverConfigs,
		"clientConfig":  clientConfig,
	})
	if err != nil {
		return nil, fmt.Errorf("创建Nacos客户端失败: %v", err)
	}

	return &NacosClient{client: configClient}, nil
}

// GetConfig 获取配置
func (n *NacosClient) GetConfig(dataId, group string) (string, error) {
	content, err := n.client.GetConfig(vo.ConfigParam{
		DataId: dataId,
		Group:  group,
	})
	if err != nil {
		return "", fmt.Errorf("获取配置失败: %v", err)
	}
	return content, nil
}

// ListenConfig 监听配置变化
func (n *NacosClient) ListenConfig(dataId, group string, callback func(namespace, group, dataId, data string)) error {
	err := n.client.ListenConfig(vo.ConfigParam{
		DataId:   dataId,
		Group:    group,
		OnChange: callback,
	})
	if err != nil {
		return fmt.Errorf("监听配置失败: %v", err)
	}
	log.Printf("开始监听配置: dataId=%s, group=%s", dataId, group)
	return nil
}
