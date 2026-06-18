package blocking

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"

	storepkg "meshtastic_mqtt_server/internal/store"
)

type Cache struct {
	mu       sync.RWMutex
	nodes    map[string]struct{}
	nodeNums map[int64]struct{}
	ips      map[string]struct{}
	cidrs    []*net.IPNet
	words    []forbiddenWordRule
}

type forbiddenWordRule struct {
	word          string
	foldedWord    string
	matchType     string
	caseSensitive bool
}

func New(store *storepkg.Store) (*Cache, error) {
	cache := &Cache{}
	if err := cache.Reload(store); err != nil {
		return nil, err
	}
	return cache, nil
}

func (c *Cache) Reload(store *storepkg.Store) error {
	if store == nil {
		return fmt.Errorf("store is required")
	}

	nodeRows, err := store.ListEnabledNodeBlocking()
	if err != nil {
		return err
	}
	ipRows, err := store.ListEnabledIPBlocking()
	if err != nil {
		return err
	}
	wordRows, err := store.ListEnabledForbiddenWordBlocking()
	if err != nil {
		return err
	}

	nodes := make(map[string]struct{}, len(nodeRows))
	nodeNums := make(map[int64]struct{}, len(nodeRows))
	for _, row := range nodeRows {
		nodeID := strings.TrimSpace(row.NodeID)
		if nodeID != "" {
			nodes[nodeID] = struct{}{}
		}
		if row.NodeNum != nil {
			nodeNums[*row.NodeNum] = struct{}{}
		}
	}

	ips := make(map[string]struct{}, len(ipRows))
	cidrs := make([]*net.IPNet, 0, len(ipRows))
	for _, row := range ipRows {
		value := strings.TrimSpace(row.IPValue)
		if value == "" {
			continue
		}
		if ip := net.ParseIP(value); ip != nil {
			ips[ip.String()] = struct{}{}
			continue
		}
		if _, ipNet, err := net.ParseCIDR(value); err == nil {
			cidrs = append(cidrs, ipNet)
		}
	}

	words := make([]forbiddenWordRule, 0, len(wordRows))
	for _, row := range wordRows {
		word := strings.TrimSpace(row.Word)
		if word == "" || row.MatchType != storepkg.ForbiddenWordMatchContains {
			continue
		}
		words = append(words, forbiddenWordRule{word: word, foldedWord: strings.ToLower(word), matchType: row.MatchType, caseSensitive: row.CaseSensitive})
	}

	c.mu.Lock()
	c.nodes = nodes
	c.nodeNums = nodeNums
	c.ips = ips
	c.cidrs = cidrs
	c.words = words
	c.mu.Unlock()
	return nil
}

func (c *Cache) IsNodeBlocked(nodeID any, nodeNum any) bool {
	if c == nil {
		return false
	}
	id, _ := nodeID.(string)
	num, hasNum := blockingInt64FromAny(nodeNum)

	c.mu.RLock()
	defer c.mu.RUnlock()
	if id != "" {
		if _, ok := c.nodes[id]; ok {
			return true
		}
	}
	if hasNum {
		_, ok := c.nodeNums[num]
		return ok
	}
	return false
}

func (c *Cache) IsIPBlocked(host string) bool {
	if c == nil {
		return false
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	if _, ok := c.ips[ip.String()]; ok {
		return true
	}
	for _, ipNet := range c.cidrs {
		if ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

func (c *Cache) FindForbiddenWord(text any) (string, bool) {
	if c == nil {
		return "", false
	}
	value, ok := text.(string)
	if !ok || value == "" {
		return "", false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	foldedText := ""
	for _, rule := range c.words {
		if rule.matchType != storepkg.ForbiddenWordMatchContains {
			continue
		}
		if rule.caseSensitive {
			if strings.Contains(value, rule.word) {
				return rule.word, true
			}
			continue
		}
		if foldedText == "" {
			foldedText = strings.ToLower(value)
		}
		if strings.Contains(foldedText, rule.foldedWord) {
			return rule.word, true
		}
	}
	return "", false
}

func blockingInt64FromAny(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int8:
		return int64(v), true
	case int16:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint:
		return int64(v), true
	case uint8:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		if v > uint64(^uint64(0)>>1) {
			return 0, false
		}
		return int64(v), true
	case float64:
		return int64(v), v == float64(int64(v))
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}
