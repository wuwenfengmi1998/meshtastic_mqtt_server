package web

import (
	"net"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/packets"

	mqttforwardpkg "meshtastic_mqtt_server/internal/mqttforward"
	storepkg "meshtastic_mqtt_server/internal/store"
)

// MQTTStatusProvider 是 web 层向上层要的"返回当前 mqtt broker 状态"接口；
// 实现一般由 main 包传入（持有真正的 mqtt.Server / 写队列 / 统计器）。
type MQTTStatusProvider interface {
	Status() AdminMQTTStatus
	// DisconnectClient 立即踢掉指定的 MQTT 客户端。clientID 不存在时返回 false。
	DisconnectClient(clientID string) bool
	// LookupClientRemoteHost 根据 clientID 查询当前连接的远端主机（不带端口），
	// 用于把该 IP 加入屏蔽表。clientID 不存在时返回空字符串与 false。
	LookupClientRemoteHost(clientID string) (string, bool)
}

// MQTTRuntimeStatus 把 mqtt.Server / 写队列 / 转发统计三个上下文打包成
// 实现 MQTTStatusProvider 的具体类型。供 main 包构造后注入 newRouter。
type MQTTRuntimeStatus struct {
	Server      *mqtt.Server
	Address     string
	TLS         bool
	Stats       *mqttforwardpkg.Stats
	ClientStats *mqttforwardpkg.ClientStats
	DBQueue     *storepkg.WriteQueue
	DedupQueue  *mqttforwardpkg.DedupQueue
}

// AdminMQTTStatus 是 admin 路由 GET /admin/mqtt-status 返回的 JSON 视图。
type AdminMQTTStatus struct {
	Running             bool              `json:"running"`
	Address             string            `json:"address"`
	TLS                 bool              `json:"tls"`
	Version             string            `json:"version"`
	Started             int64             `json:"started"`
	Uptime              int64             `json:"uptime"`
	BytesReceived       int64             `json:"bytes_received"`
	BytesSent           int64             `json:"bytes_sent"`
	ClientsConnected    int64             `json:"clients_connected"`
	ClientsDisconnected int64             `json:"clients_disconnected"`
	ClientsMaximum      int64             `json:"clients_maximum"`
	ClientsTotal        int64             `json:"clients_total"`
	MessagesReceived    int64             `json:"messages_received"`
	MessagesSent        int64             `json:"messages_sent"`
	MessagesDropped     int64             `json:"messages_dropped"`
	DBWriteQueueLength  int               `json:"db_write_queue_length"`
	DedupQueueLength    int               `json:"dedup_queue_len"`
	Retained            int64             `json:"retained"`
	Inflight            int64             `json:"inflight"`
	InflightDropped     int64             `json:"inflight_dropped"`
	Subscriptions       int64             `json:"subscriptions"`
	PacketsReceived     int64             `json:"packets_received"`
	PacketsSent         int64             `json:"packets_sent"`
	Clients             []AdminMQTTClient `json:"clients"`
}

type AdminMQTTClient struct {
	ClientID     string `json:"client_id"`
	Username     string `json:"username"`
	Listener     string `json:"listener"`
	RemoteAddr   string `json:"remote_addr"`
	PacketsIn    int64  `json:"packets_in"`  // 客户端 → 服务器
	PacketsOut   int64  `json:"packets_out"` // 服务器 → 客户端
}

// Status 实现 MQTTStatusProvider。
func (m MQTTRuntimeStatus) Status() AdminMQTTStatus {
	if m.Server == nil || m.Server.Info == nil {
		return AdminMQTTStatus{Running: false, Address: m.Address, TLS: m.TLS, DBWriteQueueLength: m.DBQueue.Len(), DedupQueueLength: m.dedupQueueLen()}
	}
	info := m.Server.Info.Clone()
	status := AdminMQTTStatus{
		Running:             true,
		Address:             m.Address,
		TLS:                 m.TLS,
		Version:             info.Version,
		Started:             info.Started,
		Uptime:              info.Uptime,
		BytesReceived:       info.BytesReceived,
		BytesSent:           info.BytesSent,
		ClientsConnected:    info.ClientsConnected,
		ClientsDisconnected: info.ClientsDisconnected,
		ClientsMaximum:      info.ClientsMaximum,
		ClientsTotal:        info.ClientsTotal,
		MessagesReceived:    info.MessagesReceived,
		MessagesSent:        m.Stats.Forwarded(),
		MessagesDropped:     m.Stats.Dropped(),
		DBWriteQueueLength:  m.DBQueue.Len(),
		DedupQueueLength:    m.dedupQueueLen(),
		Retained:            info.Retained,
		Inflight:            info.Inflight,
		InflightDropped:     info.InflightDropped,
		Subscriptions:       info.Subscriptions,
		PacketsReceived:     info.PacketsReceived,
		PacketsSent:         info.PacketsSent,
	}
	for _, client := range m.Server.Clients.GetAll() {
		if client == nil || client.Closed() {
			continue
		}
		info := mqttClientInfo(client)
		in, out := m.ClientStats.Get(info.ClientID)
		status.Clients = append(status.Clients, AdminMQTTClient{
			ClientID:   info.ClientID,
			Username:   info.Username,
			Listener:   info.Listener,
			RemoteAddr: info.RemoteAddr,
			PacketsIn:  in,
			PacketsOut: out,
		})
	}
	return status
}

// 简化版客户端信息——只解析展示所需字段，避免依赖 main 包里的辅助。
type mqttClientInfoView struct {
	ClientID   string
	Username   string
	Listener   string
	RemoteAddr string
}

func mqttClientInfo(c *mqtt.Client) mqttClientInfoView {
	if c == nil {
		return mqttClientInfoView{}
	}
	return mqttClientInfoView{
		ClientID:   c.ID,
		Username:   string(c.Properties.Username),
		Listener:   c.Net.Listener,
		RemoteAddr: c.Net.Remote,
	}
}

func (m MQTTRuntimeStatus) dedupQueueLen() int {
	if m.DedupQueue == nil {
		return 0
	}
	return m.DedupQueue.Len()
}

// DisconnectClient 实现 MQTTStatusProvider：发送 Disconnect 报文并关闭连接。
// 使用 ErrAdministrativeAction 作为断开理由，便于日志区分。
func (m MQTTRuntimeStatus) DisconnectClient(clientID string) bool {
	if m.Server == nil || clientID == "" {
		return false
	}
	client, ok := m.Server.Clients.Get(clientID)
	if !ok || client == nil {
		return false
	}
	_ = m.Server.DisconnectClient(client, packets.ErrAdministrativeAction)
	return true
}

// LookupClientRemoteHost 实现 MQTTStatusProvider：根据 clientID 查询当前连接的远端 IP。
// Net.Remote 通常是 "host:port"，这里只返回主机部分；解析失败时回退为整个值。
func (m MQTTRuntimeStatus) LookupClientRemoteHost(clientID string) (string, bool) {
	if m.Server == nil || clientID == "" {
		return "", false
	}
	client, ok := m.Server.Clients.Get(clientID)
	if !ok || client == nil {
		return "", false
	}
	remote := client.Net.Remote
	if remote == "" {
		return "", false
	}
	if host, _, err := net.SplitHostPort(remote); err == nil && host != "" {
		return host, true
	}
	return remote, true
}
