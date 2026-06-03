package main

import (
	mqtt "github.com/mochi-mqtt/server/v2"
)

type mqttStatusProvider interface {
	Status() adminMqttStatus
}

type mqttRuntimeStatus struct {
	server  *mqtt.Server
	address string
	tls     bool
	stats   *meshtasticMessageStats
}

type adminMqttStatus struct {
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
	Retained            int64             `json:"retained"`
	Inflight            int64             `json:"inflight"`
	InflightDropped     int64             `json:"inflight_dropped"`
	Subscriptions       int64             `json:"subscriptions"`
	PacketsReceived     int64             `json:"packets_received"`
	PacketsSent         int64             `json:"packets_sent"`
	Clients             []adminMqttClient `json:"clients"`
}

type adminMqttClient struct {
	ClientID   string `json:"client_id"`
	Username   string `json:"username"`
	Listener   string `json:"listener"`
	RemoteAddr string `json:"remote_addr"`
	RemoteHost string `json:"remote_host"`
	RemotePort string `json:"remote_port"`
}

func (m mqttRuntimeStatus) Status() adminMqttStatus {
	if m.server == nil || m.server.Info == nil {
		return adminMqttStatus{Running: false, Address: m.address, TLS: m.tls}
	}
	info := m.server.Info.Clone()
	status := adminMqttStatus{
		Running:             true,
		Address:             m.address,
		TLS:                 m.tls,
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
		MessagesSent:        m.stats.Forwarded(),
		MessagesDropped:     m.stats.Dropped(),
		Retained:            info.Retained,
		Inflight:            info.Inflight,
		InflightDropped:     info.InflightDropped,
		Subscriptions:       info.Subscriptions,
		PacketsReceived:     info.PacketsReceived,
		PacketsSent:         info.PacketsSent,
	}
	for _, client := range m.server.Clients.GetAll() {
		if client == nil || client.Closed() {
			continue
		}
		clientInfo := mqttClientInfoFromClient(client)
		status.Clients = append(status.Clients, adminMqttClient{
			ClientID:   clientInfo.ClientID,
			Username:   clientInfo.Username,
			Listener:   clientInfo.Listener,
			RemoteAddr: clientInfo.RemoteAddr,
			RemoteHost: clientInfo.RemoteHost,
			RemotePort: clientInfo.RemotePort,
		})
	}
	return status
}
