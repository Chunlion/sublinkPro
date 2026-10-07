package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"sublink/database"
	"sublink/models"
	"sublink/node/protocol"
	"sublink/utils"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

const realityMLKEMTestLink = "vless://12345678-1234-1234-1234-123456789abc@example.com:443?security=reality&type=tcp&pbk=test-key&sid=ab&sni=sni.example.com&fp=chrome&flow=xtls-rprx-vision#reality"

func TestRealityMLKEMNodeSettingPersistence(t *testing.T) {
	setupNodeRawAPITestDB(t)
	node := createNodeRawAPITestNode(t, models.Node{Name: "reality", LinkName: "reality", Link: realityMLKEMTestLink, Protocol: "vless"})
	for _, setting := range []string{"enable", "disable", ""} {
		response := performJSONRequest(t, UpdateNodeRawInfo, http.MethodPost, UpdateNodeRawRequest{NodeID: node.ID, Fields: map[string]any{"MihomoRealityMLKEM": setting}})
		if !strings.Contains(response.Body.String(), `"code":200`) {
			t.Fatalf("save setting: %s", response.Body.String())
		}
		var stored models.Node
		if err := database.DB.First(&stored, node.ID).Error; err != nil {
			t.Fatal(err)
		}
		cached, ok := models.GetNodeByID(node.ID)
		if !ok || stored.MihomoRealityMLKEM != setting || cached.MihomoRealityMLKEM != setting || strings.Contains(stored.Link, "mlkem") {
			t.Fatal("setting did not persist independently of link in database/cache")
		}
	}
	for _, fields := range []map[string]any{
		{"MihomoRealityMLKEM": "invalid"},
		{"MihomoRealityMLKEM": true},
		{"MihomoRealityMLKEM": "enable", "Query.Security": "tls"},
	} {
		response := performJSONRequest(t, UpdateNodeRawInfo, http.MethodPost, UpdateNodeRawRequest{NodeID: node.ID, Fields: fields})
		if strings.Contains(response.Body.String(), `"code":200`) {
			t.Fatal("invalid or non-REALITY setting accepted")
		}
	}
}

func TestRealityMLKEMManualYAMLImport(t *testing.T) {
	for _, field := range []string{"", "support-x25519mlkem768: true", "support-x25519mlkem768: false"} {
		name := field
		if name == "" {
			name = "unset"
		}
		t.Run(name, func(t *testing.T) {
			setupNodeRawAPITestDB(t)
			data := "proxies:\n  - name: reality\n    type: vless\n    server: example.com\n    port: 443\n    uuid: 12345678-1234-1234-1234-123456789abc\n    tls: true\n    reality-opts:\n      public-key: test-key\n      short-id: ab\n      " + field + "\n"
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/nodes/add", strings.NewReader(url.Values{"link": {data}}.Encode()))
			c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			NodeAdd(c)
			var stored models.Node
			if err := database.DB.First(&stored).Error; err != nil {
				t.Fatalf("YAML import: %v response=%s", err, recorder.Body.String())
			}
			assertRealityMLKEMYAML(t, stored.Link, stored.MihomoRealityMLKEMSource, field)
		})
	}
}

func assertRealityMLKEMYAML(t *testing.T, link string, value *bool, field string) {
	t.Helper()
	proxy, err := protocol.LinkToProxy(protocol.Urls{Url: link, RealityMLKEM: value}, protocol.OutputConfig{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(proxy)
	if err != nil {
		t.Fatal(err)
	}
	if field == "" {
		if strings.Contains(string(data), "support-x25519mlkem768") {
			t.Fatal("unset source became explicit")
		}
	} else if !strings.Contains(string(data), field) {
		t.Fatalf("source value lost: %s", data)
	}
}

func TestRealityMLKEMFinalTargetAndOverridePriority(t *testing.T) {
	setupClientsAPITestDB(t)
	createClientSubscriptionFixture(t, writeTestClashTemplate(t), writeTestSurgeTemplate(t), "mlkem-sub", "mlkem-token", "reality")
	var node models.Node
	if err := database.DB.First(&node).Error; err != nil {
		t.Fatal(err)
	}
	var received struct {
		Data   string `json:"data"`
		Client string `json:"client"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"par_res":"converted"}}`))
	}))
	defer server.Close()
	saveSubStoreSettings(t, server.URL, []string{"egern", "shadowrocket", "sing-box", "json", "uri", "clashmeta"})
	for _, source := range []*bool{nil, new(true), new(false)} {
		for _, setting := range []string{"", "enable", "disable"} {
			if err := database.DB.Model(&models.Node{}).Where("id = ?", node.ID).Updates(map[string]any{"link": realityMLKEMTestLink, "protocol": "vless", "mihomo_reality_mlkem_source": source, "mihomo_reality_mlkem": setting}).Error; err != nil {
				t.Fatal(err)
			}
			if err := models.InitNodeCache(); err != nil {
				t.Fatal(err)
			}
			for _, client := range []string{"clash", "mihomo", "clashmeta", "egern", "shadowrocket", "sing-box", "json", "uri", "surge", "v2ray"} {
				response := performClientRequest(t, http.MethodGet, "/c/?token=mlkem-token&client="+client)
				if response.Code != http.StatusOK {
					t.Fatalf("%s: status %d", client, response.Code)
				}
				if client == "surge" || client == "v2ray" {
					body := response.Body.String()
					if client == "v2ray" {
						body = utils.Base64Decode(body)
					}
					if strings.Contains(body, "support-x25519mlkem768") || strings.Contains(body, "MihomoRealityMLKEM") {
						t.Fatalf("field leaked to %s", client)
					}
					continue
				}
				body := received.Data
				if client == "clash" || client == "mihomo" {
					body = response.Body.String()
				}
				want := source
				if client == "clash" || client == "mihomo" || client == "clashmeta" {
					if setting == "enable" {
						want = new(true)
					}
					if setting == "disable" {
						want = new(false)
					}
				}
				var config struct {
					Proxies []protocol.Proxy `yaml:"proxies"`
				}
				if err := yaml.Unmarshal([]byte(body), &config); err != nil || len(config.Proxies) != 1 {
					t.Fatalf("%s bridge parse: %v", client, err)
				}
				got := config.Proxies[0].RealityMLKEM()
				if (got == nil) != (want == nil) || (got != nil && *got != *want) {
					t.Fatalf("%s setting=%q: wrong output boolean", client, setting)
				}
				if config.Proxies[0].Client_fingerprint != "chrome" || config.Proxies[0].Servername != "sni.example.com" || config.Proxies[0].Flow != "xtls-rprx-vision" || config.Proxies[0].Reality_opts["public-key"] != "test-key" || config.Proxies[0].Reality_opts["short-id"] != "ab" {
					t.Fatal("unrelated REALITY configuration changed")
				}
			}
			cached, ok := models.GetNodeByID(node.ID)
			if !ok || cached.MihomoRealityMLKEM != setting || (cached.MihomoRealityMLKEMSource == nil) != (source == nil) || (source != nil && *cached.MihomoRealityMLKEMSource != *source) {
				t.Fatal("rendering mutated shared node settings")
			}
		}
	}
}
