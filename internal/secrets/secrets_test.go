package secrets

import (
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	SetSecretKey("test-secret-key")
	t.Cleanup(func() { SetSecretKey("") })

	enc := Encrypt("s3cret-value")
	if enc == "s3cret-value" {
		t.Fatal("value must be encrypted when key is set")
	}
	if !strings.HasPrefix(enc, "enc:v1:") {
		t.Fatalf("unexpected prefix: %q", enc)
	}
	if got := Decrypt(enc); got != "s3cret-value" {
		t.Fatalf("round trip failed: %q", got)
	}
}

func TestPlaintextPassthroughWhenNoKey(t *testing.T) {
	SetSecretKey("")
	if Encrypt("plain") != "plain" {
		t.Error("must pass through when no key")
	}
	if Decrypt("plain") != "plain" {
		t.Error("must pass through when no key")
	}
}

func TestLegacyPlaintextStillReadable(t *testing.T) {
	SetSecretKey("k1")
	t.Cleanup(func() { SetSecretKey("") })
	// 旧数据无前缀,解密应原样返回。
	if Decrypt("legacy-plaintext") != "legacy-plaintext" {
		t.Error("legacy plaintext must be returned as-is")
	}
}

func TestWrongKeyFailsClosed(t *testing.T) {
	SetSecretKey("key-a")
	enc := Encrypt("top-secret")
	SetSecretKey("key-b")
	if Decrypt(enc) != "" {
		t.Error("decrypt with wrong key must return empty (fail closed)")
	}
	SetSecretKey("key-a")
	if Decrypt(enc) != "top-secret" {
		t.Error("decrypt with correct key must succeed")
	}
}

func TestEmptyValue(t *testing.T) {
	SetSecretKey("k")
	t.Cleanup(func() { SetSecretKey("") })
	if Encrypt("") != "" {
		t.Error("empty value stays empty")
	}
}

func TestUniqueCiphertexts(t *testing.T) {
	SetSecretKey("k")
	t.Cleanup(func() { SetSecretKey("") })
	a := Encrypt("same")
	b := Encrypt("same")
	if a == b {
		t.Error("random nonce should produce different ciphertexts")
	}
}
