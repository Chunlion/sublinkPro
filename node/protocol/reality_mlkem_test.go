package protocol

import (
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRealityMLKEMOptionalBoolConversion(t *testing.T) {
	for _, field := range []string{"", "support-x25519mlkem768: true", "support-x25519mlkem768: false"} {
		t.Run(field, func(t *testing.T) {
			var proxy Proxy
			if err := yaml.Unmarshal([]byte("name: reality\ntype: vless\nserver: example.com\nport: 443\nuuid: 12345678-1234-1234-1234-123456789abc\nnetwork: tcp\ntls: true\nservername: sni.example.com\nclient-fingerprint: chrome\nflow: xtls-rprx-vision\nreality-opts:\n  public-key: test-key\n  short-id: ab\n  "+field+"\n"), &proxy); err != nil {
				t.Fatal(err)
			}
			link, err := EncodeProxyLink(proxy)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(link, "mlkem") {
				t.Fatal("Mihomo field leaked into a VLESS share link")
			}
			converted, err := LinkToProxy(Urls{Url: link, RealityMLKEM: proxy.RealityMLKEM()}, OutputConfig{})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(converted.Reality_opts, proxy.Reality_opts) {
				t.Fatalf("reality opts changed: got %#v want %#v", converted.Reality_opts, proxy.Reality_opts)
			}
			if converted.Servername != proxy.Servername || converted.Client_fingerprint != proxy.Client_fingerprint || converted.Flow != proxy.Flow || converted.Skip_cert_verify != proxy.Skip_cert_verify {
				t.Fatal("unrelated TLS/flow fields changed")
			}
			normalized := NormalizeProxyForHash(proxy)
			if field != "" && !reflect.DeepEqual(normalized["Reality_opts"], proxy.Reality_opts) {
				t.Fatal("normalization lost explicit MLKEM boolean")
			}
			data, err := yaml.Marshal(converted)
			if err != nil {
				t.Fatal(err)
			}
			if field != "" && !strings.Contains(string(data), field) {
				t.Fatalf("YAML lost explicit boolean: %s", data)
			}
		})
	}
}

func TestRealityMLKEMIgnoresOtherProtocolsAndNonReality(t *testing.T) {
	value := true
	for _, link := range []string{
		"vless://12345678-1234-1234-1234-123456789abc@example.com:443?security=tls&type=tcp#tls",
		"vless://12345678-1234-1234-1234-123456789abc@example.com:443?security=none&type=tcp#plain",
		"trojan://test@example.com:443#trojan",
	} {
		proxy, err := LinkToProxy(Urls{Url: link, RealityMLKEM: &value}, OutputConfig{})
		if err != nil {
			t.Fatal(err)
		}
		if _, exists := proxy.Reality_opts["support-x25519mlkem768"]; exists {
			t.Fatal("MLKEM applied outside VLESS REALITY")
		}
	}
}
