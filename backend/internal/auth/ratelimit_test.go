package auth

import (
	"testing"
	"time"
)

// 达到阈值后进入锁定期，锁定期内的判断必须与"未锁定但口令错误"走上层同一条响应路径（R1.2）。
func TestLimiterLocksAfterMaxFailures(t *testing.T) {
	now := time.Unix(0, 0)
	l := NewLimiter(3, time.Minute, 10*time.Minute)
	l.now = func() time.Time { return now }

	if !l.Allowed("teacher_a|1.2.3.4") {
		t.Fatal("初始状态应允许尝试")
	}
	for i := 0; i < 3; i++ {
		l.Fail("teacher_a|1.2.3.4")
	}
	if l.Allowed("teacher_a|1.2.3.4") {
		t.Fatal("达到阈值后应进入锁定期")
	}
}

func TestLimiterWindowExpires(t *testing.T) {
	now := time.Unix(0, 0)
	l := NewLimiter(3, time.Minute, 10*time.Minute)
	l.now = func() time.Time { return now }

	l.Fail("student_a1|1.2.3.4")
	now = now.Add(2 * time.Minute)
	if !l.Allowed("student_a1|1.2.3.4") {
		t.Fatal("窗口过期后应重新允许尝试")
	}
}

func TestLimiterResetAfterSuccess(t *testing.T) {
	l := NewLimiter(3, time.Minute, 10*time.Minute)
	for i := 0; i < 3; i++ {
		l.Fail("teacher_a|1.2.3.4")
	}
	l.Reset("teacher_a|1.2.3.4")
	if !l.Allowed("teacher_a|1.2.3.4") {
		t.Fatal("登录成功后应清除计数")
	}
}
