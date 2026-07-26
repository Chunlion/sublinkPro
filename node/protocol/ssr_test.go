package protocol

import (
	"strings"
	"testing"

	"sublink/utils"

	"gopkg.in/yaml.v3"
)

// TestSSRClashYAMLParametersRoundTrip 验证 SSR 专用参数在 Clash YAML 与 SSR 链接之间不会丢失。
func TestSSRClashYAMLParametersRoundTrip(t *testing.T) {
	var config struct {
		Proxies []Proxy `yaml:"proxies"`
	}
	err := yaml.Unmarshal([]byte(`proxies:
  - name: imported-ssr
    type: ssr
    server: ssr.example.com
    port: 8388
    cipher: aes-256-cfb
    password: test-password
    protocol: auth_aes128_md5
    protocol-param: "30135:test-user-key"
    obfs: http_simple
    obfs-param: download.example.com
`), &config)
	if err != nil {
		t.Fatalf("yaml unmarshal failed: %v", err)
	}
	if len(config.Proxies) != 1 {
		t.Fatalf("proxy count = %d, want 1", len(config.Proxies))
	}

	link, err := EncodeProxyLink(config.Proxies[0])
	if err != nil {
		t.Fatalf("EncodeProxyLink failed: %v", err)
	}
	decoded, err := DecodeSSRURL(link)
	if err != nil {
		t.Fatalf("DecodeSSRURL failed: %v", err)
	}
	assertEqualString(t, "ProtocolParam", "30135:test-user-key", decoded.Qurey.Protoparam)
	assertEqualString(t, "ObfsParam", "download.example.com", decoded.Qurey.Obfsparam)

	rebuilt, err := buildSSRProxy(Urls{Url: link}, OutputConfig{})
	if err != nil {
		t.Fatalf("buildSSRProxy failed: %v", err)
	}
	data, err := yaml.Marshal(rebuilt)
	if err != nil {
		t.Fatalf("yaml marshal failed: %v", err)
	}
	for _, want := range []string{
		"protocol-param: 30135:test-user-key",
		"obfs-param: download.example.com",
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("Clash YAML output missing %q: %s", want, string(data))
		}
	}
}

// TestDecodeSSRURLMalformedQueryDoesNotPanic 回归测试：query 段缺少 '=' 分隔符
// （如裸参数名或空 query）时不能触发 index out of range panic。
func TestDecodeSSRURLMalformedQueryDoesNotPanic(t *testing.T) {
	cases := []struct {
		name    string
		payload string
	}{
		{"bare token without equals", "example.com:8388:origin:aes-256-cfb:plain:cGFzcw/?remarks"},
		{"empty query", "example.com:8388:origin:aes-256-cfb:plain:cGFzcw/?"},
		{"single valid parameter", "example.com:8388:origin:aes-256-cfb:plain:cGFzcw/?remarks=dGVzdA"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			link := "ssr://" + utils.Base64Encode(tc.payload)
			decoded, err := DecodeSSRURL(link)
			if err != nil {
				t.Fatalf("DecodeSSRURL failed: %v", err)
			}
			if decoded.Server != "example.com" {
				t.Fatalf("Server = %q, want %q", decoded.Server, "example.com")
			}
		})
	}
}
