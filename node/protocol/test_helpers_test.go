package protocol

import (
	"strconv"
	"testing"
)

func TestParsePortRejectsInvalidValues(t *testing.T) {
	tests := []string{"0", "65536", "invalid"}
	for _, raw := range tests {
		if _, _, err := parsePort(raw, 443); err == nil {
			t.Fatalf("port %q should be rejected", raw)
		}
	}

	port, raw, err := parsePort("", 443)
	if err != nil || port != 443 || raw != "443" {
		t.Fatalf("default port = (%d, %q, %v), want (443, %q, nil)", port, raw, err, "443")
	}
}

func TestURLDecodersRejectOutOfRangePorts(t *testing.T) {
	tests := []struct {
		name   string
		decode func() error
	}{
		{name: "AnyTLS", decode: func() error { _, err := DecodeAnyTLSURL("anytls://password@example.com:70000"); return err }},
		{name: "HTTP", decode: func() error { _, err := DecodeHTTPURL("http://example.com:70000"); return err }},
		{name: "Mieru", decode: func() error { _, err := DecodeMieruURL("mieru://user:password@example.com:70000"); return err }},
		{name: "Snell", decode: func() error { _, err := DecodeSnellURL("snell://psk@example.com:70000"); return err }},
		{name: "SOCKS5", decode: func() error { _, err := DecodeSocks5URL("socks5://example.com:70000"); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.decode(); err == nil {
				t.Fatal("expected out-of-range port to be rejected")
			}
		})
	}
}

// 测试辅助函数 - 用于验证编解码结果

// assertEqualString 验证两个字符串相等
func assertEqualString(t *testing.T, field string, expected, actual string) {
	t.Helper()
	if expected != actual {
		t.Errorf("%s 不匹配: 期望 [%s], 实际 [%s]", field, expected, actual)
	}
}

// assertEqualInt 验证两个整数相等
func assertEqualInt(t *testing.T, field string, expected, actual int) {
	t.Helper()
	if expected != actual {
		t.Errorf("%s 不匹配: 期望 %d, 实际 %d", field, expected, actual)
	}
}

// assertEqualFlexPort 验证 FlexPort 类型的端口
func assertEqualFlexPort(t *testing.T, field string, expected int, actual FlexPort) {
	t.Helper()
	if expected != int(actual) {
		t.Errorf("%s 不匹配: 期望 %d, 实际 %d", field, expected, int(actual))
	}
}

// assertEqualIntInterface 验证 interface{} 类型的整数（用于协议结构体的 Port 字段）
func assertEqualIntInterface(t *testing.T, field string, expected, actual any) {
	t.Helper()
	expectedInt := toInt(expected)
	actualInt := toInt(actual)
	if expectedInt != actualInt {
		t.Errorf("%s 不匹配: 期望 %d, 实际 %d", field, expectedInt, actualInt)
	}
}

// toInt 将 interface{} 转换为 int
func toInt(v any) int {
	switch val := v.(type) {
	case int:
		return val
	case float64:
		return int(val)
	case string:
		i, _ := strconv.Atoi(val)
		return i
	default:
		return 0
	}
}

// assertEqualBool 验证两个布尔值相等
func assertEqualBool(t *testing.T, field string, expected, actual bool) {
	t.Helper()
	if expected != actual {
		t.Errorf("%s 不匹配: 期望 %v, 实际 %v", field, expected, actual)
	}
}

// assertContains 验证字符串包含子串
func assertContains(t *testing.T, field string, str, substr string) {
	t.Helper()
	if len(str) == 0 || len(substr) == 0 {
		t.Errorf("%s 验证失败: 字符串或子串为空", field)
		return
	}
	found := false
	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("%s 应包含 [%s], 实际: [%s]", field, substr, str)
	}
}

func mustMap(t *testing.T, field string, value any) map[string]any {
	t.Helper()
	m, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s 类型不匹配: 期望 map[string]any, 实际 %#v", field, value)
	}
	return m
}

func mustString(t *testing.T, field string, value any) string {
	t.Helper()
	s, ok := value.(string)
	if !ok {
		t.Fatalf("%s 类型不匹配: 期望 string, 实际 %#v", field, value)
	}
	return s
}

func mustBool(t *testing.T, field string, value any) bool {
	t.Helper()
	b, ok := value.(bool)
	if !ok {
		t.Fatalf("%s 类型不匹配: 期望 bool, 实际 %#v", field, value)
	}
	return b
}

// AssertEqualString 导出版本，供其他测试包复用。
func AssertEqualString(t *testing.T, field string, expected, actual string) {
	assertEqualString(t, field, expected, actual)
}

// AssertContains 导出版本，供其他测试包复用。
func AssertContains(t *testing.T, field string, str, substr string) {
	assertContains(t, field, str, substr)
}
