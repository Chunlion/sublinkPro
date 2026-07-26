package utils

import (
	"encoding/base64"
	"testing"
)

// TestBase64DecodeStandardAndURLSafe 回归测试：Base64Decode 通过是否包含 '-'/'_'
// 区分标准与 URL-safe 编码，两种输入都必须正确解码。
func TestBase64DecodeStandardAndURLSafe(t *testing.T) {
	// "???" 的标准编码为 "Pz8/"（含 '/'），URL-safe 编码为 "Pz8_"（含 '_'）
	raw := "???"
	stdEncoded := base64.StdEncoding.EncodeToString([]byte(raw))
	urlEncoded := base64.URLEncoding.EncodeToString([]byte(raw))

	if got := Base64Decode(stdEncoded); got != raw {
		t.Fatalf("Base64Decode(%q) = %q, want %q", stdEncoded, got, raw)
	}
	if got := Base64Decode(urlEncoded); got != raw {
		t.Fatalf("Base64Decode(%q) = %q, want %q", urlEncoded, got, raw)
	}

	// 无填充的 URL-safe 输入依赖 IsBase64makeup 自动补齐
	unpadded := base64.RawURLEncoding.EncodeToString([]byte(raw))
	if got := Base64Decode(unpadded); got != raw {
		t.Fatalf("Base64Decode(%q) = %q, want %q", unpadded, got, raw)
	}
}

func TestIpFormatValidationAcceptsMultilineEntries(t *testing.T) {
	input := "192.168.0.0/16\r\n10.10.2.0/24"

	if !IpFormatValidation(input) {
		t.Fatalf("expected multiline CIDR list to be valid")
	}
}

func TestIpFormatValidationRejectsInvalidEntry(t *testing.T) {
	input := "192.168.0.0/16\nnot-an-ip"

	if IpFormatValidation(input) {
		t.Fatalf("expected invalid IP list to be rejected")
	}
}

func TestIsIpInCidrChecksAllEntries(t *testing.T) {
	allowList := "192.168.0.0/16\n10.10.2.0/24"

	if !IsIpInCidr("10.10.2.15", allowList) {
		t.Fatalf("expected IP to match the second allow-list entry")
	}

	if IsIpInCidr("10.10.3.15", allowList) {
		t.Fatalf("expected IP outside all ranges to be rejected")
	}
}

func TestIsIpInCidrSupportsExactIPsAndCommaSeparatedEntries(t *testing.T) {
	allowList := "192.168.1.10, 10.10.2.0/24"

	if !IsIpInCidr("192.168.1.10", allowList) {
		t.Fatalf("expected exact IP match to be allowed")
	}

	if !IsIpInCidr("10.10.2.42", allowList) {
		t.Fatalf("expected CIDR match to be allowed")
	}
}
