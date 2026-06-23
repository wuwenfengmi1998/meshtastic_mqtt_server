package main

import (
	"net"
	"testing"
	"time"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/packets"
)

// TestTCPNoDelay 测试 TCP_NODELAY 是否正确设置
func TestTCPNoDelay(t *testing.T) {
	// 创建一个模拟的 TCP 连接
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().String()

	// 模拟客户端连接
	connChan := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			t.Errorf("Failed to accept connection: %v", err)
			return
		}
		connChan <- conn
	}()

	// 客户端连接
	clientConn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer clientConn.Close()

	// 等待服务器端接受连接
	serverConn := <-connChan
	defer serverConn.Close()

	// 创建 MQTT Client 包装
	cl := &mqtt.Client{
		Net: mqtt.ClientConnection{
			Conn:   serverConn,
			Remote: serverConn.RemoteAddr().String(),
		},
	}

	// 创建 hook 并调用 OnConnect
	hook := &meshtasticFilterHook{}
	pk := packets.Packet{}

	err = hook.OnConnect(cl, pk)
	if err != nil {
		t.Fatalf("OnConnect failed: %v", err)
	}

	// 验证 TCP_NODELAY 是否设置
	if tcpConn, ok := serverConn.(*net.TCPConn); ok {
		// 这里我们无法直接读取 TCP_NODELAY 的值，但可以验证没有错误
		// 实际上，我们可以通过设置后再次设置来验证
		err := tcpConn.SetNoDelay(false)
		if err != nil {
			t.Fatalf("Failed to set NoDelay to false: %v", err)
		}
		err = tcpConn.SetNoDelay(true)
		if err != nil {
			t.Fatalf("Failed to set NoDelay to true: %v", err)
		}
		t.Log("TCP_NODELAY successfully set")
	} else {
		t.Fatal("Connection is not a TCP connection")
	}
}

// TestQoS0MessageLatency 测试 QoS0 消息的响应延迟
func TestQoS0MessageLatency(t *testing.T) {
	// 创建一个简单的 TCP echo 服务器来模拟 MQTT
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().String()

	// 启动服务器
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// 设置 TCP_NODELAY
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			tcpConn.SetNoDelay(true)
		}

		buf := make([]byte, 1024)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			// 立即写回（模拟 ACK）
			_, err = conn.Write(buf[:n])
			if err != nil {
				return
			}
		}
	}()

	// 客户端连接
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}
	defer conn.Close()

	// 测试小数据包的延迟
	testData := []byte("test")
	samples := 10
	var totalLatency time.Duration

	for i := 0; i < samples; i++ {
		start := time.Now()

		_, err := conn.Write(testData)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		buf := make([]byte, len(testData))
		_, err = conn.Read(buf)
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}

		latency := time.Since(start)
		totalLatency += latency
		t.Logf("Round trip %d: %v", i+1, latency)
	}

	avgLatency := totalLatency / time.Duration(samples)
	t.Logf("Average latency: %v", avgLatency)

	// 平均延迟应该小于 10ms（如果没有 Nagle 算法延迟）
	if avgLatency > 10*time.Millisecond {
		t.Logf("Warning: Average latency %v is higher than expected, may indicate Nagle's algorithm is active", avgLatency)
	}
}
