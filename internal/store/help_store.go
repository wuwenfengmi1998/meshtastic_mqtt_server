package store

import (
	"fmt"
	"strings"
)

const maxHelpMarkdownBytes = 200 * 1024

const DefaultHelpMarkdown = `## 连接地址

将 Meshtastic 设备连接到本服务提供的 MQTT broker。

- 默认地址：**mesh.gat-iot.com**
- 默认端口：**1883**
- 用户名称：**meshdev**
- 密码：**large4cats**

## 频道加密要求

为了让服务能够解析 Meshtastic MQTT payload，频道需要满足以下任一条件：

- 频道不加密。
- 使用 Meshtastic 默认 PSK：**AQ==**。

如果使用自定义加密密钥，数据可能会被判定为无法解密并丢弃。

## 反馈问题

如果遇到 bug，请在 GitHub [提交 issue](https://github.com/wuwenfengmi1998/meshtastic_mqtt_server)，或联系邮箱 [kevin@lmve.net](mailto:kevin@lmve.net)。`

func (s *Store) GetLatestHelpContent() (*HelpContentRecord, error) {
	var row HelpContentRecord
	if err := s.db.Order("id DESC").Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Store) InsertHelpContent(markdown, createdBy string) (*HelpContentRecord, error) {
	markdown = strings.TrimSpace(markdown)
	createdBy = strings.TrimSpace(createdBy)
	if markdown == "" {
		return nil, fmt.Errorf("markdown is required")
	}
	if len([]byte(markdown)) > maxHelpMarkdownBytes {
		return nil, fmt.Errorf("markdown exceeds %d bytes", maxHelpMarkdownBytes)
	}
	row := HelpContentRecord{Markdown: markdown, CreatedBy: createdBy}
	if err := s.db.Create(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}
