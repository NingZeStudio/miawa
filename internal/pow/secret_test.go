package pow

import (
	"bytes"
	"testing"
)

// TestNewManagerEmptySecretRandomized 空 HMAC 密钥必须在创建时随机生成：
// 空密钥的 HMAC 任何人都能计算，攻击者可对内存中未过期的 nonce 伪造
// difficulty=0 的挑战签名，完全绕过 PoW。
func TestNewManagerEmptySecretRandomized(t *testing.T) {
	m1 := NewManager(Config{})
	m2 := NewManager(Config{})
	if len(m1.secret) == 0 {
		t.Fatalf("空 Secret 时应随机生成密钥，实际为空")
	}
	if bytes.Equal(m1.secret, m2.secret) {
		t.Fatalf("两次创建的随机密钥不应相同")
	}
	m3 := NewManager(Config{Secret: "explicit"})
	if !bytes.Equal(m3.secret, []byte("explicit")) {
		t.Fatalf("显式提供的密钥应原样使用")
	}
}
