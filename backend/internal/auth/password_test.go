package auth

import "testing"

func TestHashAndVerify(t *testing.T) {
	h, err := NewHasher(4) // 测试用最低成本，加快运行
	if err != nil {
		t.Fatalf("构造哈希器失败: %v", err)
	}
	hash, err := h.Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("哈希失败: %v", err)
	}
	if hash == "correct horse battery staple" {
		t.Fatal("不允许明文存储口令")
	}
	if err := h.Verify(hash, "correct horse battery staple"); err != nil {
		t.Fatalf("正确口令应通过: %v", err)
	}
	if err := h.Verify(hash, "wrong"); err != ErrInvalidCredentials {
		t.Fatalf("错误口令应返回统一错误，实际 %v", err)
	}
}
