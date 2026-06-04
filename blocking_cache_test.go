package main

import "testing"

func TestBlockingCacheLoadsEnabledRules(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	nodeNum := int64(305419896)
	if _, err := st.CreateNodeBlocking("!12345678", &nodeNum, "enabled", true); err != nil {
		t.Fatalf("CreateNodeBlocking(enabled) error = %v", err)
	}
	disabledNodeNum := int64(7)
	if _, err := st.CreateNodeBlocking("!00000007", &disabledNodeNum, "disabled", false); err != nil {
		t.Fatalf("CreateNodeBlocking(disabled) error = %v", err)
	}
	if _, err := st.CreateIPBlocking("192.168.1.0/24", "lan", true); err != nil {
		t.Fatalf("CreateIPBlocking(cidr) error = %v", err)
	}
	if _, err := st.CreateIPBlocking("10.0.0.1", "disabled", false); err != nil {
		t.Fatalf("CreateIPBlocking(disabled) error = %v", err)
	}
	if _, err := st.CreateForbiddenWordBlocking("spam", "contains", false, "enabled", true); err != nil {
		t.Fatalf("CreateForbiddenWordBlocking(enabled) error = %v", err)
	}
	if _, err := st.CreateForbiddenWordBlocking("blocked", "contains", false, "disabled", false); err != nil {
		t.Fatalf("CreateForbiddenWordBlocking(disabled) error = %v", err)
	}

	cache, err := newBlockingCache(st)
	if err != nil {
		t.Fatalf("newBlockingCache() error = %v", err)
	}

	if !cache.IsNodeBlocked("!12345678", nil) {
		t.Fatal("IsNodeBlocked(enabled node id) = false, want true")
	}
	if !cache.IsNodeBlocked("", uint32(nodeNum)) {
		t.Fatal("IsNodeBlocked(enabled node num) = false, want true")
	}
	if cache.IsNodeBlocked("!00000007", disabledNodeNum) {
		t.Fatal("IsNodeBlocked(disabled node) = true, want false")
	}
	if !cache.IsIPBlocked("192.168.1.42") {
		t.Fatal("IsIPBlocked(CIDR member) = false, want true")
	}
	if cache.IsIPBlocked("10.0.0.1") {
		t.Fatal("IsIPBlocked(disabled IP) = true, want false")
	}
	if word, ok := cache.FindForbiddenWord("This is SPAM text"); !ok || word != "spam" {
		t.Fatalf("FindForbiddenWord(case-insensitive) = %q, %v, want spam, true", word, ok)
	}
	if _, ok := cache.FindForbiddenWord("disabled blocked text"); ok {
		t.Fatal("FindForbiddenWord(disabled word) = true, want false")
	}
}

func TestBlockingCacheIPExactAndCIDR(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if _, err := st.CreateIPBlocking("127.0.0.1", "loopback", true); err != nil {
		t.Fatalf("CreateIPBlocking(ip) error = %v", err)
	}
	if _, err := st.CreateIPBlocking("2001:db8::/32", "docs", true); err != nil {
		t.Fatalf("CreateIPBlocking(ipv6 cidr) error = %v", err)
	}
	cache, err := newBlockingCache(st)
	if err != nil {
		t.Fatalf("newBlockingCache() error = %v", err)
	}

	if !cache.IsIPBlocked("127.0.0.1") {
		t.Fatal("IsIPBlocked(exact IPv4) = false, want true")
	}
	if !cache.IsIPBlocked("2001:db8::1") {
		t.Fatal("IsIPBlocked(IPv6 CIDR) = false, want true")
	}
	if cache.IsIPBlocked("localhost") {
		t.Fatal("IsIPBlocked(hostname) = true, want false")
	}
}

func TestBlockingCacheForbiddenWordCaseSensitivity(t *testing.T) {
	st := openTestStore(t)
	defer st.Close()

	if _, err := st.CreateForbiddenWordBlocking("Spam", "contains", true, "case-sensitive", true); err != nil {
		t.Fatalf("CreateForbiddenWordBlocking(case-sensitive) error = %v", err)
	}
	cache, err := newBlockingCache(st)
	if err != nil {
		t.Fatalf("newBlockingCache() error = %v", err)
	}

	if _, ok := cache.FindForbiddenWord("lowercase spam"); ok {
		t.Fatal("FindForbiddenWord(lowercase) = true, want false")
	}
	if word, ok := cache.FindForbiddenWord("contains Spam"); !ok || word != "Spam" {
		t.Fatalf("FindForbiddenWord(exact case) = %q, %v, want Spam, true", word, ok)
	}
}
