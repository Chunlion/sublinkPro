package node

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sublink/database"
	"sublink/internal/testutil"
	"sublink/models"
	"sublink/node/protocol"
	"testing"

	"github.com/glebarez/sqlite"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

func TestGenerateProxyLinkReconstructsDNSStyleECH(t *testing.T) {
	proxy := protocol.Proxy{
		Name:       "导入节点-ECH-DNS",
		Type:       "vless",
		Server:     "example.com",
		Port:       443,
		Uuid:       "12345678-1234-1234-1234-123456789abc",
		Network:    "ws",
		Tls:        true,
		Servername: "example.com",
		ECH_opts: map[string]any{
			"enable":            true,
			"query-server-name": "encryptedsni.com",
		},
	}

	link := GenerateProxyLink(proxy)
	if link == "" {
		t.Fatal("生成链接失败")
	}

	if !strings.Contains(link, "ech=encryptedsni.com%2Bhttps%3A%2F%2Fdns.alidns.com%2Fdns-query") {
		t.Fatalf("ImportedECH 应包含重建后的 DNS ECH, 实际: %s", link)
	}
	decoded, err := protocol.DecodeVLESSURL(link)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}
	if decoded.Query.Ech != "encryptedsni.com+https://dns.alidns.com/dns-query" {
		t.Fatalf("RestoredECH 不匹配: 期望 %s, 实际 %s", "encryptedsni.com+https://dns.alidns.com/dns-query", decoded.Query.Ech)
	}
}

func TestGenerateProxyLinkPreservesECHConfig(t *testing.T) {
	proxy := protocol.Proxy{
		Name:       "导入节点-ECH-Config",
		Type:       "vless",
		Server:     "example.com",
		Port:       443,
		Uuid:       "12345678-1234-1234-1234-123456789abc",
		Network:    "ws",
		Tls:        true,
		Servername: "example.com",
		ECH_opts: map[string]any{
			"enable": true,
			"config": "BASE64_ECH_CONFIG",
		},
	}

	link := GenerateProxyLink(proxy)
	if link == "" {
		t.Fatal("生成链接失败")
	}

	if !strings.Contains(link, "ech=BASE64_ECH_CONFIG") {
		t.Fatalf("ImportedECHConfig 应包含 config 形式 ECH, 实际: %s", link)
	}
}

func TestGenerateProxyLinkKeepsNonVLESSUnchanged(t *testing.T) {
	proxy := protocol.Proxy{
		Name:     "trojan-node",
		Type:     "trojan",
		Server:   "example.com",
		Port:     443,
		Password: "secret",
	}

	genericLink := GenerateProxyLink(proxy)
	reconstructedLink := GenerateProxyLink(proxy)
	if genericLink != reconstructedLink {
		t.Fatalf("NonVLESSLink 不匹配: 期望 %s, 实际 %s", genericLink, reconstructedLink)
	}
}

func TestGenerateProxyLinkDoesNotReconstructDisabledECH(t *testing.T) {
	proxy := protocol.Proxy{
		Name:       "导入节点-ECH-Disabled",
		Type:       "vless",
		Server:     "example.com",
		Port:       443,
		Uuid:       "12345678-1234-1234-1234-123456789abc",
		Network:    "ws",
		Tls:        true,
		Servername: "example.com",
		ECH_opts: map[string]any{
			"enable":            false,
			"query-server-name": "encryptedsni.com",
		},
	}

	link := GenerateProxyLink(proxy)
	if link == "" {
		t.Fatal("生成链接失败")
	}
	if strings.Contains(link, "ech=") {
		t.Fatalf("禁用 ECH 时不应重建顶层 ech, 实际: %s", link)
	}
}

func TestExpandClashProxyProvidersFetchesReferencedHTTPProviderWithHeaders(t *testing.T) {
	var gotUserAgent string
	var gotCustomHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserAgent = r.Header.Get("User-Agent")
		gotCustomHeader = r.Header.Get("X-Airport-Token")
		_, _ = w.Write([]byte(`proxies:
  - name: provider-node
    type: trojan
    server: provider.example.com
    port: 443
    password: secret
`))
	}))
	defer server.Close()

	var config ClashConfig
	if err := yaml.Unmarshal([]byte(`proxy-providers:
  remote-a:
    type: http
    url: "`+server.URL+`/remote-a"
  remote-b:
    type: http
    url: "`+server.URL+`/remote-b"
proxy-groups:
  - name: auto
    type: select
    use:
      - remote-a
`), &config); err != nil {
		t.Fatalf("yaml unmarshal failed: %v", err)
	}

	err := expandClashProxyProviders(context.Background(), server.Client(), server.URL+"/root", &config, "SublinkPro-Test", models.AirportRequestHeaders{
		{Key: "X-Airport-Token", Value: "token-1"},
	})
	if err != nil {
		t.Fatalf("expandClashProxyProviders failed: %v", err)
	}
	if len(config.Proxies) != 1 {
		t.Fatalf("proxy count = %d, want 1", len(config.Proxies))
	}
	if config.Proxies[0].Name != "provider-node" {
		t.Fatalf("proxy name = %q, want provider-node", config.Proxies[0].Name)
	}
	if gotUserAgent != "SublinkPro-Test" {
		t.Fatalf("provider User-Agent = %q, want SublinkPro-Test", gotUserAgent)
	}
	if gotCustomHeader != "token-1" {
		t.Fatalf("provider custom header = %q, want token-1", gotCustomHeader)
	}
}

func TestExpandClashProxyProvidersDoesNotForwardCustomHeadersToDifferentHost(t *testing.T) {
	var gotUserAgent string
	var gotCustomHeader string
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserAgent = r.Header.Get("User-Agent")
		gotCustomHeader = r.Header.Get("X-Airport-Token")
		_, _ = w.Write([]byte(`proxies:
  - name: cross-host-provider-node
    type: trojan
    server: provider.example.com
    port: 443
    password: secret
`))
	}))
	defer providerServer.Close()

	rootServer := httptest.NewServer(http.NotFoundHandler())
	defer rootServer.Close()

	var config ClashConfig
	if err := yaml.Unmarshal([]byte(`proxy-providers:
  remote-a:
    type: http
    url: "`+providerServer.URL+`/remote-a"
`), &config); err != nil {
		t.Fatalf("yaml unmarshal failed: %v", err)
	}

	err := expandClashProxyProviders(context.Background(), providerServer.Client(), rootServer.URL+"/root", &config, "SublinkPro-Test", models.AirportRequestHeaders{
		{Key: "X-Airport-Token", Value: "token-1"},
	})
	if err != nil {
		t.Fatalf("expandClashProxyProviders failed: %v", err)
	}
	if gotUserAgent != "SublinkPro-Test" {
		t.Fatalf("provider User-Agent = %q, want SublinkPro-Test", gotUserAgent)
	}
	if gotCustomHeader != "" {
		t.Fatalf("cross-host provider custom header = %q, want empty", gotCustomHeader)
	}
}

func TestExpandClashProxyProvidersStripsCustomHeadersOnCrossHostRedirect(t *testing.T) {
	var redirectedUserAgent string
	var redirectedCustomHeader string
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectedUserAgent = r.Header.Get("User-Agent")
		redirectedCustomHeader = r.Header.Get("X-Airport-Token")
		_, _ = w.Write([]byte(`proxies:
  - name: redirected-provider-node
    type: trojan
    server: redirected.example.com
    port: 443
    password: secret
`))
	}))
	defer redirectTarget.Close()

	rootServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL+"/provider", http.StatusFound)
	}))
	defer rootServer.Close()

	var config ClashConfig
	if err := yaml.Unmarshal([]byte(`proxy-providers:
  remote-a:
    type: http
    url: "`+rootServer.URL+`/remote-a"
`), &config); err != nil {
		t.Fatalf("yaml unmarshal failed: %v", err)
	}

	err := expandClashProxyProviders(context.Background(), rootServer.Client(), rootServer.URL+"/root", &config, "SublinkPro-Test", models.AirportRequestHeaders{
		{Key: "X-Airport-Token", Value: "token-1"},
	})
	if err != nil {
		t.Fatalf("expandClashProxyProviders failed: %v", err)
	}
	if redirectedUserAgent != "SublinkPro-Test" {
		t.Fatalf("redirected User-Agent = %q, want SublinkPro-Test", redirectedUserAgent)
	}
	if redirectedCustomHeader != "" {
		t.Fatalf("redirected custom header = %q, want empty", redirectedCustomHeader)
	}
}

func TestFetchClashProxyProviderRejectsInvalidURLScheme(t *testing.T) {
	_, err := fetchClashProxyProvider(context.Background(), http.DefaultClient, "bad-scheme", "ftp://example.com/provider.yaml", "example.com", "", nil)
	if err == nil {
		t.Fatal("expected invalid scheme error, got nil")
	}
	if !strings.Contains(err.Error(), "不受支持") {
		t.Fatalf("invalid scheme error = %q, want unsupported scheme message", err.Error())
	}
}

func TestFetchClashProxyProviderRejectsInvalidRedirectScheme(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "ftp://example.com/provider.yaml", http.StatusFound)
	}))
	defer server.Close()

	_, err := fetchClashProxyProvider(context.Background(), server.Client(), "bad-redirect-scheme", server.URL+"/provider", normalizedURLHost(server.URL), "", nil)
	if err == nil {
		t.Fatal("expected invalid redirect scheme error, got nil")
	}
	if !strings.Contains(err.Error(), "ftp") || !strings.Contains(err.Error(), "不受支持") {
		t.Fatalf("invalid redirect scheme error = %q, want unsupported ftp scheme message", err.Error())
	}
}

func TestFetchClashProxyProviderRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := strings.Repeat("x", 1024*1024)
		for i := int64(0); i <= providerResponseSizeLimit/int64(len(chunk)); i++ {
			_, _ = w.Write([]byte(chunk))
		}
	}))
	defer server.Close()

	_, err := fetchClashProxyProvider(context.Background(), server.Client(), "too-large", server.URL+"/provider", normalizedURLHost(server.URL), "", nil)
	if err == nil {
		t.Fatal("expected oversized provider response error, got nil")
	}
	if !strings.Contains(err.Error(), "超过大小限制") {
		t.Fatalf("oversized response error = %q, want size limit message", err.Error())
	}
}

func TestSelectClashProxyProvidersIncludeAllVariantsSelectAllProviders(t *testing.T) {
	for _, tc := range []struct {
		name  string
		group string
	}{
		{name: "include-all", group: "include-all: true"},
		{name: "include-all-providers", group: "include-all-providers: true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var config ClashConfig
			if err := yaml.Unmarshal([]byte(`proxy-providers:
  provider-a:
    type: http
    url: "http://example.com/a"
  provider-b:
    type: http
    url: "http://example.com/b"
proxy-groups:
  - name: auto
    type: select
    use:
      - provider-a
    `+tc.group+`
`), &config); err != nil {
				t.Fatalf("yaml unmarshal failed: %v", err)
			}

			providers := selectClashProxyProviders(&config)
			gotNames := []string{providers[0].Name, providers[1].Name}
			wantNames := []string{"provider-a", "provider-b"}
			if !reflect.DeepEqual(gotNames, wantNames) {
				t.Fatalf("selected providers = %v, want %v", gotNames, wantNames)
			}
		})
	}
}

func TestExpandClashProxyProvidersRejectsTooManySelectedProviders(t *testing.T) {
	config := ClashConfig{
		ProxyProviders:     make(map[string]ClashProxyProvider, selectedProviderCountLimit+1),
		ProxyProviderOrder: make([]string, 0, selectedProviderCountLimit+1),
	}
	for i := 0; i < selectedProviderCountLimit+1; i++ {
		name := "provider-" + strconv.Itoa(i)
		config.ProxyProviderOrder = append(config.ProxyProviderOrder, name)
		config.ProxyProviders[name] = ClashProxyProvider{Type: "http", URL: "http://example.com/" + name}
	}

	err := expandClashProxyProviders(context.Background(), http.DefaultClient, "http://example.com/root", &config, "", nil)
	if err == nil {
		t.Fatal("expected selected provider count limit error, got nil")
	}
	if !strings.Contains(err.Error(), "数量过多") {
		t.Fatalf("provider count error = %q, want count limit message", err.Error())
	}
}

func TestParseClashConfigDataKeepsTopLevelProxiesWithoutFetchingProviders(t *testing.T) {
	providerFetched := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerFetched = true
		_, _ = w.Write([]byte(`proxies:
  - name: provider-node
    type: trojan
    server: provider.example.com
    port: 443
    password: secret
`))
	}))
	defer server.Close()

	config, errYaml, providerErr := parseClashConfigData(context.Background(), server.Client(), server.URL+"/root", []byte(`proxies:
  - name: inline-node
    type: trojan
    server: inline.example.com
    port: 443
    password: secret
proxy-providers:
  remote-a:
    type: http
    url: "`+server.URL+`/remote-a"
proxy-groups:
  - name: auto
    type: select
    use:
      - remote-a
`), "", nil)
	if errYaml != nil {
		t.Fatalf("yaml unmarshal failed: %v", errYaml)
	}
	if providerErr != nil {
		t.Fatalf("provider expansion should be skipped, got error: %v", providerErr)
	}
	if providerFetched {
		t.Fatal("provider URL was fetched even though top-level proxies exist")
	}
	if len(config.Proxies) != 1 {
		t.Fatalf("proxy count = %d, want 1", len(config.Proxies))
	}
	if config.Proxies[0].Name != "inline-node" {
		t.Fatalf("proxy name = %q, want inline-node", config.Proxies[0].Name)
	}
}

func TestParseClashConfigDataExpandsProviderOnlyConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`proxies:
  - name: provider-only-node
    type: trojan
    server: provider-only.example.com
    port: 443
    password: secret
  - name: provider-hy-node
    type: hysteria
    server: hy.example.com
    port: 443
    auth-str: secret-hy
    up: 11 Mbps
    down: 55 mbps
  - name: provider-hy2-node
    type: hysteria2
    server: hy2.example.com
    port: 443
    password: secret-hy2
    up-speed: 11mbps
    down-speed: 55 Mbps
`))
	}))
	defer server.Close()

	config, errYaml, providerErr := parseClashConfigData(context.Background(), server.Client(), server.URL+"/root", []byte(`proxy-providers:
  remote-a:
    type: http
    url: "`+server.URL+`/remote-a"
proxy-groups:
  - name: auto
    type: select
    use:
      - remote-a
`), "", nil)
	if errYaml != nil {
		t.Fatalf("yaml unmarshal failed: %v", errYaml)
	}
	if providerErr != nil {
		t.Fatalf("provider expansion failed: %v", providerErr)
	}
	if len(config.Proxies) != 3 {
		t.Fatalf("proxy count = %d, want 3", len(config.Proxies))
	}
	if config.Proxies[0].Name != "provider-only-node" {
		t.Fatalf("proxy name = %q, want provider-only-node", config.Proxies[0].Name)
	}
	if got := int(config.Proxies[1].Up); got != 11 {
		t.Fatalf("hysteria up = %d, want 11", got)
	}
	if got := int(config.Proxies[1].Down); got != 55 {
		t.Fatalf("hysteria down = %d, want 55", got)
	}
	if got := int(config.Proxies[2].Up_Speed); got != 11 {
		t.Fatalf("hysteria2 up-speed = %d, want 11", got)
	}
	if got := int(config.Proxies[2].Down_Speed); got != 55 {
		t.Fatalf("hysteria2 down-speed = %d, want 55", got)
	}
}

func TestExpandClashProxyProvidersFallsBackToAllHTTPProvidersAndDedupesURL(t *testing.T) {
	requestCounts := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCounts[r.URL.Path]++
		switch r.URL.Path {
		case "/a":
			_, _ = w.Write([]byte(`proxies:
  - name: provider-a
    type: trojan
    server: a.example.com
    port: 443
    password: secret-a
`))
		case "/c":
			_, _ = w.Write([]byte(`proxies:
  - name: provider-c
    type: trojan
    server: c.example.com
    port: 443
    password: secret-c
`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var config ClashConfig
	if err := yaml.Unmarshal([]byte(`proxy-providers:
  provider-a:
    type: http
    url: "`+server.URL+`/a"
  provider-a-duplicate-url:
    type: http
    url: "`+server.URL+`/a"
  provider-file:
    type: file
    path: ./local.yaml
  provider-c:
    type: http
    url: "`+server.URL+`/c"
`), &config); err != nil {
		t.Fatalf("yaml unmarshal failed: %v", err)
	}

	err := expandClashProxyProviders(context.Background(), server.Client(), server.URL+"/root", &config, "", nil)
	if err != nil {
		t.Fatalf("expandClashProxyProviders failed: %v", err)
	}
	if len(config.Proxies) != 2 {
		t.Fatalf("proxy count = %d, want 2", len(config.Proxies))
	}
	gotNames := []string{config.Proxies[0].Name, config.Proxies[1].Name}
	wantNames := []string{"provider-a", "provider-c"}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("provider order = %v, want %v", gotNames, wantNames)
	}
	if requestCounts["/a"] != 1 {
		t.Fatalf("duplicate provider URL fetched %d times, want 1", requestCounts["/a"])
	}
	if requestCounts["/c"] != 1 {
		t.Fatalf("provider /c fetched %d times, want 1", requestCounts["/c"])
	}
}

func TestExpandClashProxyProvidersReturnsProviderErrorWhenNoNodesParsed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "provider unavailable", http.StatusBadGateway)
	}))
	defer server.Close()

	var config ClashConfig
	if err := yaml.Unmarshal([]byte(`proxy-providers:
  broken:
    type: http
    url: "`+server.URL+`/broken"
`), &config); err != nil {
		t.Fatalf("yaml unmarshal failed: %v", err)
	}

	err := expandClashProxyProviders(context.Background(), server.Client(), server.URL+"/root", &config, "", nil)
	if err == nil {
		t.Fatal("expected provider error, got nil")
	}
	if !strings.Contains(err.Error(), "broken") || !strings.Contains(err.Error(), "502") {
		t.Fatalf("provider error = %q, want provider name and HTTP status", err.Error())
	}
}

func TestParseClashProviderPayloadSupportsPlainAndBase64Links(t *testing.T) {
	link := GenerateProxyLink(protocol.Proxy{
		Name:     "plain-provider-node",
		Type:     "trojan",
		Server:   "plain.example.com",
		Port:     443,
		Password: "secret",
	})
	if link == "" {
		t.Fatal("GenerateProxyLink returned empty link")
	}

	plainProxies, err := parseClashProviderPayload([]byte(link + "\n"))
	if err != nil {
		t.Fatalf("plain provider payload parse failed: %v", err)
	}
	if len(plainProxies) != 1 || plainProxies[0].Name != "plain-provider-node" {
		t.Fatalf("plain provider proxies = %+v, want one plain-provider-node", plainProxies)
	}

	base64Proxies, err := parseClashProviderPayload([]byte(base64.StdEncoding.EncodeToString([]byte(link + "\n"))))
	if err != nil {
		t.Fatalf("base64 provider payload parse failed: %v", err)
	}
	if len(base64Proxies) != 1 || base64Proxies[0].Name != "plain-provider-node" {
		t.Fatalf("base64 provider proxies = %+v, want one plain-provider-node", base64Proxies)
	}
}

func TestApplyAirportNodeNamePrefixAddsPrefixOnly(t *testing.T) {
	airport := &models.Airport{
		ID:               27,
		NodeNameUniquify: true,
	}
	proxys := []protocol.Proxy{{Name: "香港节点-01"}, {Name: "香港节点-02"}}

	result := applyAirportNodeNamePrefix(airport, proxys)
	got := []string{result[0].Name, result[1].Name}
	want := []string{"[A27]香港节点-01", "[A27]香港节点-02"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("前缀唯一化结果不匹配: got=%v want=%v", got, want)
	}
}

func TestApplyAirportNodeNamePrefixFallsBackForWhitespacePrefix(t *testing.T) {
	airport := &models.Airport{
		ID:               27,
		NodeNameUniquify: true,
		NodeNamePrefix:   "   ",
	}
	proxys := []protocol.Proxy{{Name: "香港节点-01"}}

	result := applyAirportNodeNamePrefix(airport, proxys)
	if result[0].Name != "[A27]香港节点-01" {
		t.Fatalf("空白前缀应回退到默认前缀，实际: %s", result[0].Name)
	}
}

func TestApplyAirportIntraNodeUniquifyNumbersDuplicateNamesWithinAirport(t *testing.T) {
	airport := &models.Airport{
		NodeNameIntraUniquify: true,
	}
	proxys := []protocol.Proxy{{Name: "[A27]香港节点-01"}, {Name: "[A27]香港节点-01"}, {Name: "[A27]新加坡节点-01"}, {Name: "[A27]香港节点-01"}}

	result := applyAirportIntraNodeUniquify(airport, proxys)
	got := []string{result[0].Name, result[1].Name, result[2].Name, result[3].Name}
	want := []string{"[A27]香港节点-01-1", "[A27]香港节点-01-2", "[A27]新加坡节点-01", "[A27]香港节点-01-3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("机场内编号唯一化结果不匹配: got=%v want=%v", got, want)
	}
}

func TestApplyAirportIntraNodeUniquifyCanNumberWithoutPrefix(t *testing.T) {
	airport := &models.Airport{
		NodeNameIntraUniquify: true,
	}
	proxys := []protocol.Proxy{{Name: "香港节点-01"}, {Name: "香港节点-01"}, {Name: "日本节点-01"}}

	result := applyAirportIntraNodeUniquify(airport, proxys)
	got := []string{result[0].Name, result[1].Name, result[2].Name}
	want := []string{"香港节点-01-1", "香港节点-01-2", "日本节点-01"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("无前缀时机场内编号结果不匹配: got=%v want=%v", got, want)
	}
}

func TestScheduleClashToNodeLinksPreservesCustomRemarkOnAirportSync(t *testing.T) {
	setupSubscriptionCountryBackfillTestDB(t)

	airport := &models.Airport{
		Name:     "remark-airport",
		URL:      "https://example.com/sub.yaml",
		CronExpr: "0 */12 * * *",
		Enabled:  true,
		Group:    "default",
	}
	if err := airport.Add(); err != nil {
		t.Fatalf("add airport: %v", err)
	}

	originalProxy := protocol.Proxy{
		Name:     "jp-01",
		Type:     "ss",
		Server:   "jp.example.com",
		Port:     8388,
		Cipher:   "aes-128-gcm",
		Password: "password",
	}
	existingNode := createExistingSubscriptionNode(t, airport.ID, airport.Name, originalProxy, "", 1)
	if err := models.UpdateNodeFields(existingNode.ID, map[string]any{
		"name":      "my-jp-remark",
		"name_mode": models.NodeNameModeLink,
	}); err != nil {
		t.Fatalf("customize node remark: %v", err)
	}

	updatedProxy := originalProxy
	updatedProxy.Name = "jp-02"
	changedNodeIDs, err := scheduleClashToNodeLinks(context.Background(), airport.ID, []protocol.Proxy{updatedProxy}, airport.Name, nil, nil)
	if err != nil {
		t.Fatalf("sync subscription: %v", err)
	}

	nodes, err := models.ListBySourceID(airport.ID)
	if err != nil {
		t.Fatalf("list source nodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("source node count = %d, want 1", len(nodes))
	}
	if nodes[0].ID != existingNode.ID {
		t.Fatalf("node ID = %d, want existing ID %d", nodes[0].ID, existingNode.ID)
	}
	if nodes[0].Name != "my-jp-remark" {
		t.Fatalf("node remark = %q, want custom remark", nodes[0].Name)
	}
	if nodes[0].LinkName != "jp-02" {
		t.Fatalf("node link name = %q, want updated upstream name", nodes[0].LinkName)
	}
	if len(changedNodeIDs) != 1 || changedNodeIDs[0] != existingNode.ID {
		t.Fatalf("changed IDs = %v, want [%d]", changedNodeIDs, existingNode.ID)
	}
}

func TestScheduleClashToNodeLinksKeepsRemarkModeNameWhenItMatchesOldUpstreamName(t *testing.T) {
	setupSubscriptionCountryBackfillTestDB(t)

	airport := &models.Airport{
		Name:     "locked-remark-airport",
		URL:      "https://example.com/sub.yaml",
		CronExpr: "0 */12 * * *",
		Enabled:  true,
		Group:    "default",
	}
	if err := airport.Add(); err != nil {
		t.Fatalf("add airport: %v", err)
	}

	originalProxy := protocol.Proxy{
		Name:     "hk-locked",
		Type:     "ss",
		Server:   "hk-locked.example.com",
		Port:     8388,
		Cipher:   "aes-128-gcm",
		Password: "password",
	}
	existingNode := createExistingSubscriptionNode(t, airport.ID, airport.Name, originalProxy, "", 1)
	if err := models.UpdateNodeFields(existingNode.ID, map[string]any{
		"name_mode": models.NodeNameModeRemark,
	}); err != nil {
		t.Fatalf("lock node remark mode: %v", err)
	}

	updatedProxy := originalProxy
	updatedProxy.Name = "hk-upstream-renamed"
	changedNodeIDs, err := scheduleClashToNodeLinks(context.Background(), airport.ID, []protocol.Proxy{updatedProxy}, airport.Name, nil, nil)
	if err != nil {
		t.Fatalf("sync subscription: %v", err)
	}

	nodes, err := models.ListBySourceID(airport.ID)
	if err != nil {
		t.Fatalf("list source nodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("source node count = %d, want 1", len(nodes))
	}
	if nodes[0].ID != existingNode.ID {
		t.Fatalf("node ID = %d, want existing ID %d", nodes[0].ID, existingNode.ID)
	}
	if nodes[0].Name != "hk-locked" || nodes[0].NameMode != models.NodeNameModeRemark {
		t.Fatalf("locked remark was overwritten: Name=%q NameMode=%q", nodes[0].Name, nodes[0].NameMode)
	}
	if nodes[0].LinkName != "hk-upstream-renamed" {
		t.Fatalf("node link name = %q, want updated upstream name", nodes[0].LinkName)
	}
	if len(changedNodeIDs) != 1 || changedNodeIDs[0] != existingNode.ID {
		t.Fatalf("changed IDs = %v, want [%d]", changedNodeIDs, existingNode.ID)
	}
}

func TestScheduleClashToNodeLinksMatchesSingleRenamedNodeWhenContentHashChanges(t *testing.T) {
	setupSubscriptionCountryBackfillTestDB(t)

	airport := &models.Airport{
		Name:     "single-node-rename-airport",
		URL:      "https://example.com/sub.yaml",
		CronExpr: "0 */12 * * *",
		Enabled:  true,
		Group:    "default",
	}
	if err := airport.Add(); err != nil {
		t.Fatalf("add airport: %v", err)
	}

	originalProxy := protocol.Proxy{
		Name:     "Japan A",
		Type:     "ss",
		Server:   "single-rename.example.com",
		Port:     8388,
		Cipher:   "aes-128-gcm",
		Password: "password",
	}
	existingNode := createExistingSubscriptionNode(t, airport.ID, airport.Name, originalProxy, "", 1)
	if err := models.UpdateNodeFields(existingNode.ID, map[string]any{
		"name":         "JP manual remark",
		"name_mode":    models.NodeNameModeRemark,
		"content_hash": "legacy-name-sensitive-hash",
	}); err != nil {
		t.Fatalf("customize existing node: %v", err)
	}

	updatedProxy := originalProxy
	updatedProxy.Name = "Japan A new"
	changedNodeIDs, err := scheduleClashToNodeLinks(context.Background(), airport.ID, []protocol.Proxy{updatedProxy}, airport.Name, nil, nil)
	if err != nil {
		t.Fatalf("sync subscription: %v", err)
	}

	nodes, err := models.ListBySourceID(airport.ID)
	if err != nil {
		t.Fatalf("list source nodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("source node count = %d, want 1", len(nodes))
	}
	if nodes[0].ID != existingNode.ID {
		t.Fatalf("node ID = %d, want existing ID %d", nodes[0].ID, existingNode.ID)
	}
	if nodes[0].Name != "JP manual remark" || nodes[0].NameMode != models.NodeNameModeRemark {
		t.Fatalf("manual remark was not preserved: Name=%q NameMode=%q", nodes[0].Name, nodes[0].NameMode)
	}
	if nodes[0].LinkName != "Japan A new" {
		t.Fatalf("link name = %q, want updated upstream name", nodes[0].LinkName)
	}
	if len(changedNodeIDs) != 1 || changedNodeIDs[0] != existingNode.ID {
		t.Fatalf("changed IDs = %v, want [%d]", changedNodeIDs, existingNode.ID)
	}
}

func TestScheduleClashToNodeLinksPreservesHysteria2RemarkWhenRenamedAndContentHashChanges(t *testing.T) {
	setupSubscriptionCountryBackfillTestDB(t)

	airport := &models.Airport{
		Name:     "hy2-stable-sync-airport",
		URL:      "https://example.com/sub.yaml",
		CronExpr: "0 */12 * * *",
		Enabled:  true,
		Group:    "default",
	}
	if err := airport.Add(); err != nil {
		t.Fatalf("add airport: %v", err)
	}

	originalProxy := protocol.Proxy{
		Name:          "hy2-old-name",
		Type:          "hysteria2",
		Server:        "hy2-sync.example.com",
		Port:          443,
		Password:      "hy2-password",
		Auth_str:      "hy2-password",
		Sni:           "hy2.example.com",
		Obfs:          "salamander",
		Obfs_password: "obfs-password",
		Alpn:          []string{"h3"},
	}
	otherProxy := protocol.Proxy{
		Name:     "ss-unchanged",
		Type:     "ss",
		Server:   "ss-unchanged.example.com",
		Port:     8388,
		Cipher:   "aes-128-gcm",
		Password: "ss-password",
	}
	targetNode := createExistingSubscriptionNode(t, airport.ID, airport.Name, originalProxy, "", 1)
	otherNode := createExistingSubscriptionNode(t, airport.ID, airport.Name, otherProxy, "", 2)
	if err := models.UpdateNodeFields(targetNode.ID, map[string]any{
		"name":      "hy2 manual remark",
		"name_mode": models.NodeNameModeRemark,
	}); err != nil {
		t.Fatalf("customize hy2 node remark: %v", err)
	}

	updatedProxy := originalProxy
	updatedProxy.Name = "hy2-new-upstream-name"
	updatedProxy.Up = protocol.Mbps(20)
	updatedProxy.Down = protocol.Mbps(80)
	newContentHash := protocol.GenerateProxyContentHash(updatedProxy)
	if targetNode.ContentHash == newContentHash {
		t.Fatalf("test setup content hash did not change: %q", newContentHash)
	}
	updatedLink := GenerateProxyLink(updatedProxy)
	if updatedLink == "" {
		t.Fatal("GenerateProxyLink returned empty updated HY2 link")
	}
	oldIdentity := subscriptionNodeStableIdentityKey(targetNode.Link)
	newIdentity := subscriptionNodeStableIdentityKey(updatedLink)
	if oldIdentity == "" || oldIdentity != newIdentity {
		t.Fatalf("HY2 stable identity mismatch:\nold=%s\nnew=%s", oldIdentity, newIdentity)
	}

	changedNodeIDs, err := scheduleClashToNodeLinks(context.Background(), airport.ID, []protocol.Proxy{otherProxy, updatedProxy}, airport.Name, nil, nil)
	if err != nil {
		t.Fatalf("sync subscription: %v", err)
	}

	nodes, err := models.ListBySourceID(airport.ID)
	if err != nil {
		t.Fatalf("list source nodes: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("source node count = %d, want 2", len(nodes))
	}

	var storedTarget, storedOther *models.Node
	for i := range nodes {
		switch nodes[i].ID {
		case targetNode.ID:
			storedTarget = &nodes[i]
		case otherNode.ID:
			storedOther = &nodes[i]
		}
	}
	if storedTarget == nil {
		t.Fatalf("HY2 old node ID %d was deleted or replaced; nodes=%+v", targetNode.ID, nodes)
	}
	if storedOther == nil {
		t.Fatalf("unchanged node ID %d was deleted or replaced; nodes=%+v", otherNode.ID, nodes)
	}
	if storedTarget.Name != "hy2 manual remark" {
		t.Fatalf("HY2 Name = %q, want manual remark", storedTarget.Name)
	}
	if storedTarget.LinkName != "hy2-new-upstream-name" {
		t.Fatalf("HY2 LinkName = %q, want updated upstream name", storedTarget.LinkName)
	}
	if storedTarget.NameMode != models.NodeNameModeRemark {
		t.Fatalf("HY2 NameMode = %q, want %q", storedTarget.NameMode, models.NodeNameModeRemark)
	}
	if storedTarget.ContentHash != newContentHash {
		t.Fatalf("HY2 ContentHash = %q, want %q", storedTarget.ContentHash, newContentHash)
	}

	foundTargetChange := false
	for _, id := range changedNodeIDs {
		if id == targetNode.ID {
			foundTargetChange = true
			break
		}
	}
	if !foundTargetChange {
		t.Fatalf("changed IDs = %v, want to include HY2 node ID %d", changedNodeIDs, targetNode.ID)
	}
}

func TestScheduleClashToNodeLinksFallsBackToUniqueSourceSort(t *testing.T) {
	setupSubscriptionCountryBackfillTestDB(t)

	airport := &models.Airport{
		Name:     "source-sort-fallback-airport",
		URL:      "https://example.com/sub.yaml",
		CronExpr: "0 */12 * * *",
		Enabled:  true,
		Group:    "default",
	}
	if err := airport.Add(); err != nil {
		t.Fatalf("add airport: %v", err)
	}

	firstProxy := protocol.Proxy{
		Name:     "sort-a",
		Type:     "ss",
		Server:   "sort-a.example.com",
		Port:     8388,
		Cipher:   "aes-128-gcm",
		Password: "old-password",
	}
	secondProxy := protocol.Proxy{
		Name:     "sort-b",
		Type:     "ss",
		Server:   "sort-b.example.com",
		Port:     8388,
		Cipher:   "aes-128-gcm",
		Password: "second-password",
	}
	firstExistingNode := createExistingSubscriptionNode(t, airport.ID, airport.Name, firstProxy, "", 1)
	secondExistingNode := createExistingSubscriptionNode(t, airport.ID, airport.Name, secondProxy, "", 2)
	if err := models.UpdateNodeFields(firstExistingNode.ID, map[string]any{
		"name":      "sort-a manual",
		"name_mode": models.NodeNameModeRemark,
	}); err != nil {
		t.Fatalf("customize first node: %v", err)
	}

	changedFirstProxy := firstProxy
	changedFirstProxy.Name = "sort-a-renamed"
	changedFirstProxy.Password = "rotated-password"
	changedNodeIDs, err := scheduleClashToNodeLinks(context.Background(), airport.ID, []protocol.Proxy{changedFirstProxy, secondProxy}, airport.Name, nil, nil)
	if err != nil {
		t.Fatalf("sync subscription: %v", err)
	}

	nodes, err := models.ListBySourceID(airport.ID)
	if err != nil {
		t.Fatalf("list source nodes: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("source node count = %d, want 2", len(nodes))
	}
	var storedFirst, storedSecond *models.Node
	for i := range nodes {
		switch nodes[i].ID {
		case firstExistingNode.ID:
			storedFirst = &nodes[i]
		case secondExistingNode.ID:
			storedSecond = &nodes[i]
		}
	}
	if storedFirst == nil || storedSecond == nil {
		t.Fatalf("expected both original node IDs to be preserved; nodes=%+v", nodes)
	}
	if storedFirst.Name != "sort-a manual" || storedFirst.NameMode != models.NodeNameModeRemark {
		t.Fatalf("manual remark was not preserved: Name=%q NameMode=%q", storedFirst.Name, storedFirst.NameMode)
	}
	if storedFirst.LinkName != "sort-a-renamed" || storedFirst.SourceSort != 1 {
		t.Fatalf("first node state = LinkName:%q SourceSort:%d, want sort-a-renamed/1", storedFirst.LinkName, storedFirst.SourceSort)
	}
	if storedSecond.LinkName != "sort-b" || storedSecond.SourceSort != 2 {
		t.Fatalf("second node state = LinkName:%q SourceSort:%d, want sort-b/2", storedSecond.LinkName, storedSecond.SourceSort)
	}
	if len(changedNodeIDs) != 1 || changedNodeIDs[0] != firstExistingNode.ID {
		t.Fatalf("changed IDs = %v, want [%d]", changedNodeIDs, firstExistingNode.ID)
	}
}

func TestSubscriptionNodeMatcherStableIdentityMatchIsNotDeleted(t *testing.T) {
	oldProxy := protocol.Proxy{
		Name:     "Japan A",
		Type:     "ss",
		Server:   "matcher-stable.example.com",
		Port:     8388,
		Cipher:   "aes-128-gcm",
		Password: "password",
	}
	oldLink := GenerateProxyLink(oldProxy)
	if oldLink == "" {
		t.Fatal("GenerateProxyLink returned empty old link")
	}

	newProxy := oldProxy
	newProxy.Name = "Japan A new"
	newLink := GenerateProxyLink(newProxy)
	if newLink == "" {
		t.Fatal("GenerateProxyLink returned empty new link")
	}
	newHash := protocol.GenerateProxyContentHash(newProxy)
	currentIdentityCounts := map[string]int{subscriptionNodeStableIdentityKey(newLink): 1}
	oldNode := models.Node{
		ID:          101,
		Name:        "JP manual remark",
		LinkName:    "Japan A",
		NameMode:    models.NodeNameModeRemark,
		Link:        oldLink,
		SourceSort:  1,
		ContentHash: "legacy-content-hash",
	}
	matcher := newSubscriptionNodeMatcher([]models.Node{oldNode}, map[string]int{newHash: 1}, currentIdentityCounts)

	matchedNode, ok := matcher.match(newLink, newHash, newProxy.Name, 1)
	if !ok {
		t.Fatal("matcher did not match stable identity")
	}
	if matchedNode.ID != oldNode.ID {
		t.Fatalf("matched node ID = %d, want %d", matchedNode.ID, oldNode.ID)
	}
	if !matcher.isMatched(oldNode.ID) {
		t.Fatalf("matched old node ID %d was not recorded", oldNode.ID)
	}
	nodesToDelete := subscriptionNodeIDsToDelete(map[int]models.Node{oldNode.ID: oldNode}, matcher)
	if len(nodesToDelete) != 0 {
		t.Fatalf("nodesToDelete = %v, want empty", nodesToDelete)
	}
}

func TestSubscriptionNodeMatcherStableIdentityMatchesAddedProtocolsOnRename(t *testing.T) {
	cases := []struct {
		name  string
		proxy protocol.Proxy
	}{
		{
			name: "hysteria",
			proxy: protocol.Proxy{
				Type:     "hysteria",
				Server:   "hy-stable.example.com",
				Port:     443,
				Auth_str: "hy-auth",
				Protocol: "udp",
				Peer:     "hy.example.com",
				Alpn:     []string{"h3"},
			},
		},
		{
			name: "hysteria2",
			proxy: protocol.Proxy{
				Type:          "hysteria2",
				Server:        "hy2-stable.example.com",
				Port:          443,
				Password:      "hy2-password",
				Auth_str:      "hy2-password",
				Sni:           "hy2.example.com",
				Obfs:          "salamander",
				Obfs_password: "obfs-password",
				Alpn:          []string{"h3"},
			},
		},
		{
			name: "tuic",
			proxy: protocol.Proxy{
				Type:                  "tuic",
				Server:                "tuic-stable.example.com",
				Port:                  443,
				Uuid:                  "12345678-1234-1234-1234-123456789abc",
				Password:              "tuic-password",
				Version:               5,
				Tls:                   true,
				Sni:                   "tuic.example.com",
				Congestion_controller: "bbr",
				Udp_relay_mode:        "native",
				Alpn:                  []string{"h3"},
			},
		},
		{
			name: "ssr",
			proxy: protocol.Proxy{
				Type:          "ssr",
				Server:        "ssr-stable.example.com",
				Port:          8388,
				Cipher:        "aes-128-gcm",
				Password:      "ssr-password",
				Protocol:      "auth_sha1_v4",
				Obfs:          "tls1.2_ticket_auth",
				Obfs_password: "cdn.example.com",
			},
		},
		{
			name: "wireguard",
			proxy: protocol.Proxy{
				Type:           "wireguard",
				Server:         "wg-stable.example.com",
				Port:           51820,
				Private_key:    "client-private-key",
				Public_key:     "server-public-key",
				Pre_shared_key: "pre-shared-key",
				Ip:             "10.0.0.2",
				Mtu:            1280,
				Reserved:       []int{1, 2, 3},
			},
		},
		{
			name: "anytls",
			proxy: protocol.Proxy{
				Type:     "anytls",
				Server:   "anytls-stable.example.com",
				Port:     443,
				Password: "anytls-password",
				Sni:      "anytls.example.com",
				Alpn:     []string{"h2", "http/1.1"},
			},
		},
		{
			name: "http",
			proxy: protocol.Proxy{
				Type:   "http",
				Server: "http-stable.example.com",
				Port:   8080,
			},
		},
		{
			name: "https",
			proxy: protocol.Proxy{
				Type:     "https",
				Server:   "https-stable.example.com",
				Port:     8443,
				Username: "https-user",
				Password: "https-password",
				Tls:      true,
				Sni:      "https.example.com",
			},
		},
		{
			name: "socks5",
			proxy: protocol.Proxy{
				Type:     "socks5",
				Server:   "socks-stable.example.com",
				Port:     1080,
				Username: "socks-user",
				Password: "socks-password",
			},
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oldProxy := tc.proxy
			oldProxy.Name = tc.name + " old"
			oldLink := GenerateProxyLink(oldProxy)
			if oldLink == "" {
				t.Fatal("GenerateProxyLink returned empty old link")
			}

			newProxy := oldProxy
			newProxy.Name = tc.name + " renamed"
			newLink := GenerateProxyLink(newProxy)
			if newLink == "" {
				t.Fatal("GenerateProxyLink returned empty new link")
			}

			oldKey := subscriptionNodeStableIdentityKey(oldLink)
			newKey := subscriptionNodeStableIdentityKey(newLink)
			if oldKey == "" || newKey == "" {
				t.Fatalf("stable identity key missing: old=%q new=%q", oldKey, newKey)
			}
			if oldKey != newKey {
				t.Fatalf("stable identity key changed after rename:\nold=%s\nnew=%s", oldKey, newKey)
			}

			newHash := protocol.GenerateProxyContentHash(newProxy)
			oldNode := models.Node{
				ID:          201 + i,
				Name:        tc.name + " manual remark",
				LinkName:    oldProxy.Name,
				NameMode:    models.NodeNameModeRemark,
				Link:        oldLink,
				SourceSort:  1,
				ContentHash: "legacy-content-hash-" + tc.name,
			}
			matcher := newSubscriptionNodeMatcher([]models.Node{oldNode}, map[string]int{newHash: 1}, map[string]int{newKey: 1})

			matchedNode, ok := matcher.match(newLink, newHash, newProxy.Name, 1)
			if !ok {
				t.Fatal("matcher did not match stable identity")
			}
			if matchedNode.ID != oldNode.ID {
				t.Fatalf("matched node ID = %d, want %d", matchedNode.ID, oldNode.ID)
			}

			nodesToDelete := subscriptionNodeIDsToDelete(map[int]models.Node{oldNode.ID: oldNode}, matcher)
			if len(nodesToDelete) != 0 {
				t.Fatalf("nodesToDelete = %v, want empty", nodesToDelete)
			}
		})
	}
}

func TestScheduleClashToNodeLinksDeletesOnlyUnmatchedStaleNode(t *testing.T) {
	setupSubscriptionCountryBackfillTestDB(t)

	airport := &models.Airport{
		Name:     "delete-unmatched-airport",
		URL:      "https://example.com/sub.yaml",
		CronExpr: "0 */12 * * *",
		Enabled:  true,
		Group:    "default",
	}
	if err := airport.Add(); err != nil {
		t.Fatalf("add airport: %v", err)
	}

	keptProxy := protocol.Proxy{
		Name:     "keep-a",
		Type:     "ss",
		Server:   "keep-a.example.com",
		Port:     8388,
		Cipher:   "aes-128-gcm",
		Password: "keep-password",
	}
	staleProxy := protocol.Proxy{
		Name:     "stale-b",
		Type:     "ss",
		Server:   "stale-b.example.com",
		Port:     8388,
		Cipher:   "aes-128-gcm",
		Password: "stale-password",
	}
	keptNode := createExistingSubscriptionNode(t, airport.ID, airport.Name, keptProxy, "", 1)
	staleNode := createExistingSubscriptionNode(t, airport.ID, airport.Name, staleProxy, "", 2)
	if err := models.UpdateNodeFields(keptNode.ID, map[string]any{
		"name":         "keep manual",
		"name_mode":    models.NodeNameModeRemark,
		"content_hash": "legacy-keep-hash",
	}); err != nil {
		t.Fatalf("customize kept node: %v", err)
	}

	renamedKeptProxy := keptProxy
	renamedKeptProxy.Name = "keep-a-renamed"
	changedNodeIDs, err := scheduleClashToNodeLinks(context.Background(), airport.ID, []protocol.Proxy{renamedKeptProxy}, airport.Name, nil, nil)
	if err != nil {
		t.Fatalf("sync subscription: %v", err)
	}

	nodes, err := models.ListBySourceID(airport.ID)
	if err != nil {
		t.Fatalf("list source nodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("source node count = %d, want 1", len(nodes))
	}
	if nodes[0].ID != keptNode.ID {
		t.Fatalf("remaining node ID = %d, want kept node ID %d; stale node ID was %d", nodes[0].ID, keptNode.ID, staleNode.ID)
	}
	if nodes[0].Name != "keep manual" || nodes[0].NameMode != models.NodeNameModeRemark {
		t.Fatalf("kept manual remark was not preserved: Name=%q NameMode=%q", nodes[0].Name, nodes[0].NameMode)
	}
	if nodes[0].LinkName != "keep-a-renamed" {
		t.Fatalf("link name = %q, want keep-a-renamed", nodes[0].LinkName)
	}
	if len(changedNodeIDs) != 1 || changedNodeIDs[0] != keptNode.ID {
		t.Fatalf("changed IDs = %v, want [%d]", changedNodeIDs, keptNode.ID)
	}
	var stale models.Node
	if err := database.DB.First(&stale, staleNode.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("stale node lookup error = %v, want record not found", err)
	}
}

func TestScheduleClashToNodeLinksPreservesCustomRemarkForSameHashRenamedNode(t *testing.T) {
	setupSubscriptionCountryBackfillTestDB(t)

	airport := &models.Airport{
		Name:     "same-hash-airport",
		URL:      "https://example.com/sub.yaml",
		CronExpr: "0 */12 * * *",
		Enabled:  true,
		Group:    "default",
	}
	if err := airport.Add(); err != nil {
		t.Fatalf("add airport: %v", err)
	}

	baseProxy := protocol.Proxy{
		Name:     "slot-a",
		Type:     "ss",
		Server:   "same.example.com",
		Port:     8388,
		Cipher:   "aes-128-gcm",
		Password: "password",
	}
	firstExistingNode := createExistingSubscriptionNode(t, airport.ID, airport.Name, baseProxy, "", 1)
	secondProxy := baseProxy
	secondProxy.Name = "slot-b"
	secondExistingNode := createExistingSubscriptionNode(t, airport.ID, airport.Name, secondProxy, "", 2)
	if firstExistingNode.ContentHash != secondExistingNode.ContentHash {
		t.Fatalf("test setup content hashes differ: %q != %q", firstExistingNode.ContentHash, secondExistingNode.ContentHash)
	}
	if err := models.UpdateNodeFields(firstExistingNode.ID, map[string]any{
		"name":      "custom-slot-a",
		"name_mode": models.NodeNameModeLink,
	}); err != nil {
		t.Fatalf("customize node remark: %v", err)
	}

	renamedFirstProxy := baseProxy
	renamedFirstProxy.Name = "slot-a-renamed"
	changedNodeIDs, err := scheduleClashToNodeLinks(context.Background(), airport.ID, []protocol.Proxy{renamedFirstProxy, secondProxy}, airport.Name, nil, nil)
	if err != nil {
		t.Fatalf("sync subscription: %v", err)
	}

	nodes, err := models.ListBySourceID(airport.ID)
	if err != nil {
		t.Fatalf("list source nodes: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("source node count = %d, want 2", len(nodes))
	}

	var storedFirst, storedSecond *models.Node
	for i := range nodes {
		switch nodes[i].ID {
		case firstExistingNode.ID:
			storedFirst = &nodes[i]
		case secondExistingNode.ID:
			storedSecond = &nodes[i]
		}
	}
	if storedFirst == nil {
		t.Fatalf("custom remark node ID %d was not preserved; nodes=%+v", firstExistingNode.ID, nodes)
	}
	if storedSecond == nil {
		t.Fatalf("unchanged same-hash node ID %d was not preserved; nodes=%+v", secondExistingNode.ID, nodes)
	}
	if storedFirst.Name != "custom-slot-a" {
		t.Fatalf("node remark = %q, want custom remark", storedFirst.Name)
	}
	if storedFirst.LinkName != "slot-a-renamed" {
		t.Fatalf("node link name = %q, want updated upstream name", storedFirst.LinkName)
	}
	if storedSecond.LinkName != "slot-b" {
		t.Fatalf("second node link name = %q, want slot-b", storedSecond.LinkName)
	}
	if len(changedNodeIDs) != 1 || changedNodeIDs[0] != firstExistingNode.ID {
		t.Fatalf("changed IDs = %v, want [%d]", changedNodeIDs, firstExistingNode.ID)
	}
}

func TestScheduleClashToNodeLinksKeepsRemarkWhenSameHashRenameCollidesWithDeletedNode(t *testing.T) {
	setupSubscriptionCountryBackfillTestDB(t)

	airport := &models.Airport{
		Name:     "same-hash-collision-airport",
		URL:      "https://example.com/sub.yaml",
		CronExpr: "0 */12 * * *",
		Enabled:  true,
		Group:    "default",
	}
	if err := airport.Add(); err != nil {
		t.Fatalf("add airport: %v", err)
	}

	baseProxy := protocol.Proxy{
		Name:     "slot-a",
		Type:     "ss",
		Server:   "same-collision.example.com",
		Port:     8388,
		Cipher:   "aes-128-gcm",
		Password: "password",
	}
	remarkNode := createExistingSubscriptionNode(t, airport.ID, airport.Name, baseProxy, "", 1)
	removedProxy := baseProxy
	removedProxy.Name = "slot-b"
	removedNode := createExistingSubscriptionNode(t, airport.ID, airport.Name, removedProxy, "", 2)
	if remarkNode.ContentHash != removedNode.ContentHash {
		t.Fatalf("test setup content hashes differ: %q != %q", remarkNode.ContentHash, removedNode.ContentHash)
	}
	if err := models.UpdateNodeFields(remarkNode.ID, map[string]any{
		"name":      "custom-slot-a",
		"name_mode": models.NodeNameModeRemark,
	}); err != nil {
		t.Fatalf("customize node remark: %v", err)
	}

	renamedProxy := baseProxy
	renamedProxy.Name = "slot-b"
	changedNodeIDs, err := scheduleClashToNodeLinks(context.Background(), airport.ID, []protocol.Proxy{renamedProxy}, airport.Name, nil, nil)
	if err != nil {
		t.Fatalf("sync subscription: %v", err)
	}

	nodes, err := models.ListBySourceID(airport.ID)
	if err != nil {
		t.Fatalf("list source nodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("source node count = %d, want 1", len(nodes))
	}
	if nodes[0].ID != remarkNode.ID {
		t.Fatalf("node ID = %d, want remark node ID %d; removed node ID was %d", nodes[0].ID, remarkNode.ID, removedNode.ID)
	}
	if nodes[0].Name != "custom-slot-a" || nodes[0].NameMode != models.NodeNameModeRemark {
		t.Fatalf("remark node was not preserved: Name=%q NameMode=%q", nodes[0].Name, nodes[0].NameMode)
	}
	if nodes[0].LinkName != "slot-b" {
		t.Fatalf("node link name = %q, want renamed upstream name", nodes[0].LinkName)
	}
	if len(changedNodeIDs) != 1 || changedNodeIDs[0] != remarkNode.ID {
		t.Fatalf("changed IDs = %v, want [%d]", changedNodeIDs, remarkNode.ID)
	}
}

func TestScheduleClashToNodeLinksPreservesExactLinkWhenSameHashNodesReordered(t *testing.T) {
	setupSubscriptionCountryBackfillTestDB(t)

	airport := &models.Airport{
		Name:     "same-hash-reorder-airport",
		URL:      "https://example.com/sub.yaml",
		CronExpr: "0 */12 * * *",
		Enabled:  true,
		Group:    "default",
	}
	if err := airport.Add(); err != nil {
		t.Fatalf("add airport: %v", err)
	}

	baseProxy := protocol.Proxy{
		Name:     "slot-a",
		Type:     "ss",
		Server:   "same-reorder.example.com",
		Port:     8388,
		Cipher:   "aes-128-gcm",
		Password: "password",
	}
	firstExistingNode := createExistingSubscriptionNode(t, airport.ID, airport.Name, baseProxy, "", 1)
	secondProxy := baseProxy
	secondProxy.Name = "slot-b"
	secondExistingNode := createExistingSubscriptionNode(t, airport.ID, airport.Name, secondProxy, "", 2)
	if firstExistingNode.ContentHash != secondExistingNode.ContentHash {
		t.Fatalf("test setup content hashes differ: %q != %q", firstExistingNode.ContentHash, secondExistingNode.ContentHash)
	}
	if err := models.UpdateNodeFields(firstExistingNode.ID, map[string]any{
		"name":      "custom-slot-a",
		"name_mode": models.NodeNameModeRemark,
	}); err != nil {
		t.Fatalf("customize node remark: %v", err)
	}

	renamedFirstProxy := baseProxy
	renamedFirstProxy.Name = "slot-a-renamed"
	changedNodeIDs, err := scheduleClashToNodeLinks(context.Background(), airport.ID, []protocol.Proxy{secondProxy, renamedFirstProxy}, airport.Name, nil, nil)
	if err != nil {
		t.Fatalf("sync subscription: %v", err)
	}

	nodes, err := models.ListBySourceID(airport.ID)
	if err != nil {
		t.Fatalf("list source nodes: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("source node count = %d, want 2", len(nodes))
	}

	var storedFirst, storedSecond *models.Node
	for i := range nodes {
		switch nodes[i].ID {
		case firstExistingNode.ID:
			storedFirst = &nodes[i]
		case secondExistingNode.ID:
			storedSecond = &nodes[i]
		}
	}
	if storedFirst == nil || storedSecond == nil {
		t.Fatalf("expected both original node IDs to be preserved; nodes=%+v", nodes)
	}
	if storedFirst.Name != "custom-slot-a" || storedFirst.NameMode != models.NodeNameModeRemark {
		t.Fatalf("custom remark node not preserved: Name=%q NameMode=%q", storedFirst.Name, storedFirst.NameMode)
	}
	if storedFirst.LinkName != "slot-a-renamed" || storedFirst.SourceSort != 2 {
		t.Fatalf("custom node upstream state = LinkName:%q SourceSort:%d, want slot-a-renamed/2", storedFirst.LinkName, storedFirst.SourceSort)
	}
	if storedSecond.LinkName != "slot-b" || storedSecond.SourceSort != 1 {
		t.Fatalf("exact-link node upstream state = LinkName:%q SourceSort:%d, want slot-b/1", storedSecond.LinkName, storedSecond.SourceSort)
	}
	if len(changedNodeIDs) != 2 {
		t.Fatalf("changed IDs = %v, want both reordered nodes", changedNodeIDs)
	}
}

func TestScheduleClashToNodeLinksPreservesRemarkWhenExistingContentHashMissing(t *testing.T) {
	setupSubscriptionCountryBackfillTestDB(t)

	airport := &models.Airport{
		Name:     "missing-hash-airport",
		URL:      "https://example.com/sub.yaml",
		CronExpr: "0 */12 * * *",
		Enabled:  true,
		Group:    "default",
	}
	if err := airport.Add(); err != nil {
		t.Fatalf("add airport: %v", err)
	}

	proxy := protocol.Proxy{
		Name:     "hk-01",
		Type:     "ss",
		Server:   "hk.example.com",
		Port:     8388,
		Cipher:   "aes-128-gcm",
		Password: "password",
	}
	existingNode := createExistingSubscriptionNode(t, airport.ID, airport.Name, proxy, "", 1)
	if err := database.DB.Model(&models.Node{}).Where("id = ?", existingNode.ID).Update("content_hash", "").Error; err != nil {
		t.Fatalf("clear content hash: %v", err)
	}
	existingNode.ContentHash = ""
	models.UpdateNodeCache(existingNode.ID, existingNode)

	if err := models.UpdateNodeFields(existingNode.ID, map[string]any{
		"name":      "custom-hk-remark",
		"name_mode": models.NodeNameModeRemark,
	}); err != nil {
		t.Fatalf("customize node remark: %v", err)
	}

	changedNodeIDs, err := scheduleClashToNodeLinks(context.Background(), airport.ID, []protocol.Proxy{proxy}, airport.Name, nil, nil)
	if err != nil {
		t.Fatalf("sync subscription: %v", err)
	}

	nodes, err := models.ListBySourceID(airport.ID)
	if err != nil {
		t.Fatalf("list source nodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("source node count = %d, want 1", len(nodes))
	}
	if nodes[0].ID != existingNode.ID {
		t.Fatalf("node ID = %d, want existing ID %d", nodes[0].ID, existingNode.ID)
	}
	if nodes[0].Name != "custom-hk-remark" || nodes[0].NameMode != models.NodeNameModeRemark {
		t.Fatalf("node remark not preserved: Name=%q NameMode=%q", nodes[0].Name, nodes[0].NameMode)
	}
	wantHash := protocol.GenerateProxyContentHash(proxy)
	if nodes[0].ContentHash != wantHash {
		t.Fatalf("cached content hash = %q, want %q", nodes[0].ContentHash, wantHash)
	}
	var stored models.Node
	if err := database.DB.First(&stored, existingNode.ID).Error; err != nil {
		t.Fatalf("reload node: %v", err)
	}
	if stored.ContentHash != wantHash {
		t.Fatalf("stored content hash = %q, want %q", stored.ContentHash, wantHash)
	}
	if len(changedNodeIDs) != 1 || changedNodeIDs[0] != existingNode.ID {
		t.Fatalf("changed IDs = %v, want [%d]", changedNodeIDs, existingNode.ID)
	}
}

func TestGenerateProxyLinkRoundTripsMieruClashYAML(t *testing.T) {
	var config ClashConfig
	if err := yaml.Unmarshal([]byte(`proxies:
  - name: mieru-import
    type: mieru
    server: mieru.example.com
    port-range: 2090-2099
    transport: TCP
    username: user
    password: password
    multiplexing: MULTIPLEXING_LOW
    traffic-pattern: dGVzdA==
`), &config); err != nil {
		t.Fatalf("yaml unmarshal failed: %v", err)
	}
	if len(config.Proxies) != 1 {
		t.Fatalf("proxy count = %d, want 1", len(config.Proxies))
	}

	link := GenerateProxyLink(config.Proxies[0])
	if link == "" {
		t.Fatal("GenerateProxyLink returned empty link")
	}
	decoded, err := protocol.DecodeMieruURL(link)
	if err != nil {
		t.Fatalf("DecodeMieruURL failed: %v", err)
	}
	if decoded.PortRange != "2090-2099" {
		t.Fatalf("port range = %q, want 2090-2099", decoded.PortRange)
	}
	if decoded.Transport != "TCP" {
		t.Fatalf("transport = %q, want TCP", decoded.Transport)
	}
	if decoded.Multiplexing != "MULTIPLEXING_LOW" {
		t.Fatalf("multiplexing = %q, want MULTIPLEXING_LOW", decoded.Multiplexing)
	}
	if decoded.TrafficPattern != "dGVzdA==" {
		t.Fatalf("traffic pattern = %q, want dGVzdA==", decoded.TrafficPattern)
	}
}

func TestScheduleClashToNodeLinksBackfillsEveryExistingEmptyCountryNode(t *testing.T) {
	setupSubscriptionCountryBackfillTestDB(t)

	airport := &models.Airport{
		Name:                    "回填机场",
		URL:                     "https://example.com/sub.yaml",
		CronExpr:                "0 */12 * * *",
		Enabled:                 true,
		Group:                   "默认组",
		BackfillExistingCountry: true,
	}
	if err := airport.Add(); err != nil {
		t.Fatalf("add airport: %v", err)
	}
	createCountryBackfillRule(t, "HK", "香港", "香港|HK")
	createCountryBackfillRule(t, "JP", "日本", "日本|JP")

	ordinaryProxy := protocol.Proxy{Name: "HK 普通节点", Type: "trojan", Server: "ordinary.example.com", Port: 443, Password: "secret-ordinary"}
	infoHKProxy := protocol.Proxy{Name: "HK 到期时间", Type: "trojan", Server: "info.example.com", Port: 443, Password: "secret-info"}
	infoJPProxy := protocol.Proxy{Name: "JP 剩余流量", Type: "trojan", Server: "info.example.com", Port: 443, Password: "secret-info"}
	keptCountryProxy := protocol.Proxy{Name: "JP 已有国家", Type: "trojan", Server: "kept.example.com", Port: 443, Password: "secret-kept"}

	ordinary := createExistingSubscriptionNode(t, airport.ID, airport.Name, ordinaryProxy, "", 1)
	infoHK := createExistingSubscriptionNode(t, airport.ID, airport.Name, infoHKProxy, "", 2)
	infoJP := createExistingSubscriptionNode(t, airport.ID, airport.Name, infoJPProxy, "", 3)
	keptCountry := createExistingSubscriptionNode(t, airport.ID, airport.Name, keptCountryProxy, "US", 4)

	_, err := scheduleClashToNodeLinks(context.Background(), airport.ID, []protocol.Proxy{
		ordinaryProxy,
		infoHKProxy,
		infoJPProxy,
		keptCountryProxy,
	}, airport.Name, nil, nil)
	if err != nil {
		t.Fatalf("schedule clash nodes: %v", err)
	}

	assertNodeCountry(t, ordinary.ID, "HK")
	assertNodeCountry(t, infoHK.ID, "HK")
	assertNodeCountry(t, infoJP.ID, "JP")
	assertNodeCountry(t, keptCountry.ID, "US")
	assertCachedNodeCountry(t, airport.ID, ordinary.ID, "HK")
	assertCachedNodeCountry(t, airport.ID, infoHK.ID, "HK")
	assertCachedNodeCountry(t, airport.ID, infoJP.ID, "JP")
}

func setupSubscriptionCountryBackfillTestDB(t *testing.T) {
	t.Helper()

	oldDB := database.DB
	oldDialect := database.Dialect
	oldInitialized := database.IsInitialized

	db, err := gorm.Open(sqlite.Open(testutil.UniqueMemoryDSN(t, "subscription_country_backfill_test")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := db.AutoMigrate(&models.Airport{}, &models.Node{}, &models.SubcriptionNode{}, &models.CountryRule{}, &models.SystemSetting{}); err != nil {
		t.Fatalf("auto migrate test db: %v", err)
	}

	database.DB = db
	database.Dialect = database.DialectSQLite
	database.IsInitialized = false
	if err := models.InitAirportCache(); err != nil {
		t.Fatalf("init airport cache: %v", err)
	}
	if err := models.InitNodeCache(); err != nil {
		t.Fatalf("init node cache: %v", err)
	}
	if err := models.InitCountryRuleCache(); err != nil {
		t.Fatalf("init country rule cache: %v", err)
	}
	if err := models.InitSettingCache(); err != nil {
		t.Fatalf("init setting cache: %v", err)
	}

	t.Cleanup(func() {
		database.DB = oldDB
		database.Dialect = oldDialect
		database.IsInitialized = oldInitialized
		testutil.CloseDB(t, db)
	})
}

func createCountryBackfillRule(t *testing.T, code string, name string, pattern string) {
	t.Helper()
	rule := &models.CountryRule{CountryCode: code, CountryName: name, Pattern: pattern, Priority: 100, Enabled: true}
	if err := rule.Add(); err != nil {
		t.Fatalf("add country rule %s: %v", code, err)
	}
}

func createExistingSubscriptionNode(t *testing.T, sourceID int, source string, proxy protocol.Proxy, country string, sourceSort int) models.Node {
	t.Helper()
	link := GenerateProxyLink(proxy)
	if link == "" {
		t.Fatalf("generate link for %s", proxy.Name)
	}
	node := models.Node{
		Name:        proxy.Name,
		LinkName:    proxy.Name,
		NameMode:    models.NodeNameModeLink,
		Link:        link,
		LinkAddress: proxy.Server + ":" + strconv.Itoa(int(proxy.Port)),
		LinkHost:    proxy.Server,
		LinkPort:    strconv.Itoa(int(proxy.Port)),
		LinkCountry: country,
		Protocol:    proxy.Type,
		Source:      source,
		SourceID:    sourceID,
		SourceSort:  sourceSort,
		ContentHash: protocol.GenerateProxyContentHash(proxy),
	}
	if err := node.Add(); err != nil {
		t.Fatalf("add existing node %s: %v", proxy.Name, err)
	}
	return node
}

func assertNodeCountry(t *testing.T, id int, want string) {
	t.Helper()
	var node models.Node
	if err := database.DB.First(&node, id).Error; err != nil {
		t.Fatalf("load node %d: %v", id, err)
	}
	if node.LinkCountry != want {
		t.Fatalf("node %s country = %q, want %q", node.Name, node.LinkCountry, want)
	}
}

func assertCachedNodeCountry(t *testing.T, sourceID int, nodeID int, want string) {
	t.Helper()
	nodes, err := models.ListBySourceID(sourceID)
	if err != nil {
		t.Fatalf("list cached nodes: %v", err)
	}
	for _, node := range nodes {
		if node.ID == nodeID {
			if node.LinkCountry != want {
				t.Fatalf("cached node %s country = %q, want %q", node.Name, node.LinkCountry, want)
			}
			return
		}
	}
	t.Fatalf("cached node %d not found", nodeID)
}
