package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"
	"gorm.io/gorm"
)

const (
	mqttForwardDirectionTargetToSource = "target_to_source"
	mqttForwardLoopTTL                 = 15 * time.Second
	mqttForwardLoopMaxEntries          = 10000
)

type mqttForwardReloader interface {
	ReloadForwarder(id uint64) error
	StopForwarder(id uint64)
	Status() []mqttForwardRuntimeStatus
}

type mqttForwardManager struct {
	store   *store
	mu      sync.Mutex
	runners map[uint64]*mqttForwardRunner
}

type mqttForwardRuntimeStatus struct {
	ForwarderID       uint64     `json:"forwarder_id"`
	Running           bool       `json:"running"`
	SourceConnected   bool       `json:"source_connected"`
	TargetConnected   bool       `json:"target_connected"`
	LastError         string     `json:"last_error"`
	StartedAt         *time.Time `json:"started_at"`
	MessagesForwarded uint64     `json:"messages_forwarded"`
	MessagesDropped   uint64     `json:"messages_dropped"`
}

type mqttForwardRunner struct {
	config mqttForwarderConfig
	ctx    context.Context
	cancel context.CancelFunc
	source pahomqtt.Client
	target pahomqtt.Client

	mu                sync.Mutex
	lastError         string
	startedAt         time.Time
	sourceConnected   bool
	targetConnected   bool
	messagesForwarded uint64
	messagesDropped   uint64
	loopCache         map[string]time.Time
}

func newMQTTForwardManager(store *store) *mqttForwardManager {
	return &mqttForwardManager{store: store, runners: make(map[uint64]*mqttForwardRunner)}
}

func (m *mqttForwardManager) StartFromStore() error {
	configs, err := m.store.ListEnabledMQTTForwarderConfigs()
	if err != nil {
		return err
	}
	for _, cfg := range configs {
		if len(cfg.Topics) == 0 {
			continue
		}
		runner := newMQTTForwardRunner(cfg)
		runner.Start()
		m.mu.Lock()
		m.runners[cfg.Forwarder.ID] = runner
		m.mu.Unlock()
	}
	return nil
}

func (m *mqttForwardManager) ReloadForwarder(id uint64) error {
	m.StopForwarder(id)
	cfg, err := m.store.GetMQTTForwarderConfig(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !cfg.Forwarder.Enabled || len(cfg.Topics) == 0 {
		return nil
	}
	runner := newMQTTForwardRunner(*cfg)
	runner.Start()
	m.mu.Lock()
	m.runners[id] = runner
	m.mu.Unlock()
	return nil
}

func (m *mqttForwardManager) StopForwarder(id uint64) {
	m.mu.Lock()
	runner := m.runners[id]
	delete(m.runners, id)
	m.mu.Unlock()
	if runner != nil {
		runner.Stop()
	}
}

func (m *mqttForwardManager) StopAll() {
	m.mu.Lock()
	runners := make([]*mqttForwardRunner, 0, len(m.runners))
	for id, runner := range m.runners {
		runners = append(runners, runner)
		delete(m.runners, id)
	}
	m.mu.Unlock()
	for _, runner := range runners {
		runner.Stop()
	}
}

func (m *mqttForwardManager) Status() []mqttForwardRuntimeStatus {
	m.mu.Lock()
	runners := make([]*mqttForwardRunner, 0, len(m.runners))
	for _, runner := range m.runners {
		runners = append(runners, runner)
	}
	m.mu.Unlock()
	items := make([]mqttForwardRuntimeStatus, 0, len(runners))
	for _, runner := range runners {
		items = append(items, runner.Status())
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ForwarderID < items[j].ForwarderID })
	return items
}

func newMQTTForwardRunner(config mqttForwarderConfig) *mqttForwardRunner {
	ctx, cancel := context.WithCancel(context.Background())
	return &mqttForwardRunner{config: config, ctx: ctx, cancel: cancel, startedAt: time.Now(), loopCache: make(map[string]time.Time)}
}

func (r *mqttForwardRunner) Start() {
	r.source = r.newClient(true)
	r.target = r.newClient(false)
	r.connectClient(r.target, "target")
	r.connectClient(r.source, "source")
}

func (r *mqttForwardRunner) Stop() {
	r.cancel()
	if r.source != nil && r.source.IsConnected() {
		r.source.Disconnect(250)
	}
	if r.target != nil && r.target.IsConnected() {
		r.target.Disconnect(250)
	}
}

func (r *mqttForwardRunner) Status() mqttForwardRuntimeStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	started := r.startedAt
	return mqttForwardRuntimeStatus{
		ForwarderID:       r.config.Forwarder.ID,
		Running:           true,
		SourceConnected:   r.sourceConnected,
		TargetConnected:   r.targetConnected,
		LastError:         r.lastError,
		StartedAt:         &started,
		MessagesForwarded: r.messagesForwarded,
		MessagesDropped:   r.messagesDropped,
	}
}

func (r *mqttForwardRunner) newClient(source bool) pahomqtt.Client {
	forwarder := r.config.Forwarder
	host, port, username, password, clientID, useTLS := forwarder.SourceHost, forwarder.SourcePort, forwarder.SourceUsername, forwarder.SourcePassword, forwarder.SourceClientID, forwarder.SourceTLS
	role := "source"
	if !source {
		host, port, username, password, clientID, useTLS = forwarder.TargetHost, forwarder.TargetPort, forwarder.TargetUsername, forwarder.TargetPassword, forwarder.TargetClientID, forwarder.TargetTLS
		role = "target"
	}
	if clientID == "" {
		clientID = fmt.Sprintf("mesh-forward-%d-%s", forwarder.ID, role)
	}
	scheme := "tcp"
	if useTLS {
		scheme = "ssl"
	}
	opts := pahomqtt.NewClientOptions().
		AddBroker(fmt.Sprintf("%s://%s", scheme, net.JoinHostPort(host, fmt.Sprint(port)))).
		SetClientID(clientID).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetKeepAlive(60 * time.Second).
		SetConnectionLostHandler(func(_ pahomqtt.Client, err error) {
			r.setConnected(source, false)
			r.setError(fmt.Sprintf("%s connection lost: %v", role, err))
		}).
		SetOnConnectHandler(func(client pahomqtt.Client) {
			r.setConnected(source, true)
			r.subscribe(client, source)
		})
	if username != "" {
		opts.SetUsername(username)
	}
	if password != "" {
		opts.SetPassword(password)
	}
	if useTLS {
		opts.SetTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12})
	}
	return pahomqtt.NewClient(opts)
}

func (r *mqttForwardRunner) connectClient(client pahomqtt.Client, label string) {
	token := client.Connect()
	if !token.WaitTimeout(2 * time.Second) {
		r.setError(label + " connect pending")
		return
	}
	if err := token.Error(); err != nil {
		r.setError(fmt.Sprintf("%s connect failed: %v", label, err))
	}
}

func (r *mqttForwardRunner) subscribe(client pahomqtt.Client, source bool) {
	for _, topic := range r.config.Topics {
		filter := topic.Topic
		if !source {
			if topic.Direction != mqttForwardDirectionBidirectional {
				continue
			}
			filter = mapMQTTForwardTopic(topic.Topic, topic.SourcePrefix, topic.TargetPrefix)
		}
		topicRule := topic
		token := client.Subscribe(filter, byte(topic.QoS), func(_ pahomqtt.Client, msg pahomqtt.Message) {
			r.forwardMessage(source, topicRule, msg)
		})
		if !token.WaitTimeout(2 * time.Second) {
			r.setError("subscribe pending: " + filter)
			continue
		}
		if err := token.Error(); err != nil {
			r.setError(fmt.Sprintf("subscribe %s failed: %v", filter, err))
		}
	}
}

func (r *mqttForwardRunner) forwardMessage(fromSource bool, rule mqttForwardTopicRecord, msg pahomqtt.Message) {
	if r.ctx.Err() != nil {
		return
	}
	fromTopic := msg.Topic()
	if fromSource {
		if !mqttTopicFilterMatches(rule.Topic, fromTopic) {
			return
		}
	} else if !mqttTopicFilterMatches(mapMQTTForwardTopic(rule.Topic, rule.SourcePrefix, rule.TargetPrefix), fromTopic) {
		return
	}
	toTopic := fromTopic
	forwardDirection := mqttForwardDirectionSourceToTarget
	if fromSource {
		toTopic = mapMQTTForwardTopic(fromTopic, rule.SourcePrefix, rule.TargetPrefix)
	} else {
		forwardDirection = mqttForwardDirectionTargetToSource
		toTopic = mapMQTTForwardTopic(fromTopic, rule.TargetPrefix, rule.SourcePrefix)
	}
	if r.isSuppressed(forwardDirection, fromTopic, toTopic, msg.Payload(), rule.QoS, rule.Retain) {
		r.incDropped()
		return
	}
	target := r.target
	reverseDirection := mqttForwardDirectionTargetToSource
	if !fromSource {
		target = r.source
		reverseDirection = mqttForwardDirectionSourceToTarget
	}
	r.markSuppressed(reverseDirection, toTopic, fromTopic, msg.Payload(), rule.QoS, rule.Retain)
	token := target.Publish(toTopic, byte(rule.QoS), rule.Retain, msg.Payload())
	if !token.WaitTimeout(2 * time.Second) {
		r.setError("publish pending: " + toTopic)
		r.incDropped()
		return
	}
	if err := token.Error(); err != nil {
		r.setError(fmt.Sprintf("publish %s failed: %v", toTopic, err))
		r.incDropped()
		return
	}
	r.incForwarded()
}

func (r *mqttForwardRunner) setConnected(source bool, connected bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if source {
		r.sourceConnected = connected
	} else {
		r.targetConnected = connected
	}
}

func (r *mqttForwardRunner) setError(message string) {
	r.mu.Lock()
	r.lastError = message
	r.mu.Unlock()
}

func (r *mqttForwardRunner) incForwarded() {
	r.mu.Lock()
	r.messagesForwarded++
	r.mu.Unlock()
}

func (r *mqttForwardRunner) incDropped() {
	r.mu.Lock()
	r.messagesDropped++
	r.mu.Unlock()
}

func (r *mqttForwardRunner) isSuppressed(direction, fromTopic, toTopic string, payload []byte, qos int, retain bool) bool {
	key := mqttForwardLoopKey(direction, fromTopic, toTopic, payload, qos, retain)
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	expires, ok := r.loopCache[key]
	if !ok {
		return false
	}
	if now.After(expires) {
		delete(r.loopCache, key)
		return false
	}
	delete(r.loopCache, key)
	return true
}

func (r *mqttForwardRunner) markSuppressed(direction, fromTopic, toTopic string, payload []byte, qos int, retain bool) {
	key := mqttForwardLoopKey(direction, fromTopic, toTopic, payload, qos, retain)
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.loopCache) >= mqttForwardLoopMaxEntries {
		for existing, expires := range r.loopCache {
			if now.After(expires) || len(r.loopCache) >= mqttForwardLoopMaxEntries {
				delete(r.loopCache, existing)
			}
			if len(r.loopCache) < mqttForwardLoopMaxEntries {
				break
			}
		}
	}
	r.loopCache[key] = now.Add(mqttForwardLoopTTL)
}

func mqttForwardLoopKey(direction, fromTopic, toTopic string, payload []byte, qos int, retain bool) string {
	sum := sha256.Sum256(payload)
	return fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%t\x00%s", direction, fromTopic, toTopic, qos, retain, hex.EncodeToString(sum[:]))
}

func mapMQTTForwardTopic(topic, fromPrefix, toPrefix string) string {
	fromPrefix = strings.Trim(fromPrefix, "/")
	toPrefix = strings.Trim(toPrefix, "/")
	if fromPrefix == "" {
		return topic
	}
	if topic == fromPrefix {
		return toPrefix
	}
	if strings.HasPrefix(topic, fromPrefix+"/") {
		if toPrefix == "" {
			return strings.TrimPrefix(topic, fromPrefix+"/")
		}
		return toPrefix + strings.TrimPrefix(topic, fromPrefix)
	}
	return topic
}

func mqttTopicFilterMatches(filter, topic string) bool {
	filterParts := strings.Split(filter, "/")
	topicParts := strings.Split(topic, "/")
	for i, filterPart := range filterParts {
		if filterPart == "#" {
			return i == len(filterParts)-1
		}
		if i >= len(topicParts) {
			return false
		}
		if filterPart == "+" {
			continue
		}
		if filterPart != topicParts[i] {
			return false
		}
	}
	return len(filterParts) == len(topicParts)
}
