package mqttauth

import (
	"testing"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/listeners"
)

// TestBrokerIntegration 用真实 TCP broker + paho 客户端验证认证全链路。
func TestBrokerIntegration(t *testing.T) {
	hook := NewHook(Config{
		Enabled: true,
		Users:   []User{{Username: "mesh", PasswordHash: hash(t, "secret")}},
		MaxFailures: 3,
	})
	t.Cleanup(func() { _ = hook.Stop() })

	server := mqtt.New(&mqtt.Options{InlineClient: true})
	if err := server.AddHook(hook, nil); err != nil {
		t.Fatalf("add hook: %v", err)
	}
	addr := "127.0.0.1:18883"
	if err := server.AddListener(listeners.NewTCP(listeners.Config{ID: "t", Address: addr})); err != nil {
		t.Fatalf("listen: %v", err)
	}
	if err := server.Serve(); err != nil {
		t.Fatalf("serve: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	time.Sleep(200 * time.Millisecond)

	connect := func(user, pass string) bool {
		opts := paho.NewClientOptions().
			AddBroker("tcp://" + addr).
			SetClientID("smoke-" + user + time.Now().Format("150405.000")).
			SetUsername(user).SetPassword(pass).
			SetConnectTimeout(3 * time.Second)
		client := paho.NewClient(opts)
		token := client.Connect()
		token.WaitTimeout(5 * time.Second)
		ok := token.Error() == nil
		if ok {
			client.Disconnect(50)
		}
		return ok
	}

	if !connect("mesh", "secret") {
		t.Error("valid credentials should connect")
	}
	if connect("mesh", "wrong") {
		t.Error("wrong password should be rejected")
	}
	if connect("", "") {
		t.Error("anonymous should be rejected when disabled")
	}

	// 连续失败触发封禁后,正确凭据也应被拒绝(默认阈值 3,本例 MaxFailures=3)。
	// 前面已失败 2 次(mesh/wrong 与匿名),再失败 1 次触发封禁。
	if connect("mesh", "wrong") {
		t.Fatal("wrong password should be rejected")
	}
	if connect("mesh", "secret") {
		t.Error("blocked ip must reject even valid credentials")
	}
}
