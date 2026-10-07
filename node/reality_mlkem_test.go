package node

import (
	"context"
	"testing"

	"sublink/database"
	"sublink/models"
	"sublink/node/protocol"

	"gopkg.in/yaml.v3"
)

func TestRealityMLKEMImportAndSubscriptionRefresh(t *testing.T) {
	for _, setting := range []string{"", "enable", "disable"} {
		name := setting
		if name == "" {
			name = "keep"
		}
		t.Run(name, func(t *testing.T) {
			setupSubscriptionCountryBackfillTestDB(t)
			airport := &models.Airport{Name: "mlkem-airport", URL: "https://example.com/sub", CronExpr: "0 */12 * * *", Enabled: true}
			if err := airport.Add(); err != nil {
				t.Fatal(err)
			}
			var nodeID int
			for _, field := range []string{"", "support-x25519mlkem768: true", "support-x25519mlkem768: false", ""} {
				var proxy protocol.Proxy
				if err := yaml.Unmarshal([]byte("name: reality\ntype: vless\nserver: example.com\nport: 443\nuuid: 12345678-1234-1234-1234-123456789abc\ntls: true\nnetwork: tcp\nreality-opts:\n  public-key: test-key\n  short-id: ab\n  "+field+"\n"), &proxy); err != nil {
					t.Fatal(err)
				}
				if _, err := scheduleClashToNodeLinks(context.Background(), airport.ID, []protocol.Proxy{proxy}, airport.Name, nil, nil); err != nil {
					t.Fatal(err)
				}
				nodes, err := models.ListBySourceID(airport.ID)
				if err != nil || len(nodes) != 1 {
					t.Fatalf("list nodes: count=%d err=%v", len(nodes), err)
				}
				if nodeID == 0 {
					nodeID = nodes[0].ID
					if err := models.UpdateNodeFields(nodeID, map[string]any{"mihomo_reality_mlkem": setting}); err != nil {
						t.Fatal(err)
					}
				} else if nodes[0].ID != nodeID || nodes[0].MihomoRealityMLKEM != setting {
					t.Fatal("refresh replaced the node or lost explicit override")
				}
				var stored models.Node
				if err := database.DB.First(&stored, nodeID).Error; err != nil {
					t.Fatal(err)
				}
				if stored.MihomoRealityMLKEM != setting || !equalOptionalBool(stored.MihomoRealityMLKEMSource, proxy.RealityMLKEM()) || !equalOptionalBool(nodes[0].MihomoRealityMLKEMSource, proxy.RealityMLKEM()) {
					t.Fatal("database/cache lost source tri-state or override")
				}
				converted, err := protocol.LinkToProxy(protocol.Urls{Url: stored.Link, RealityMLKEM: stored.MihomoRealityMLKEMSource}, protocol.OutputConfig{})
				if err != nil || !equalOptionalBool(converted.RealityMLKEM(), proxy.RealityMLKEM()) {
					t.Fatalf("internal node conversion lost source: %v", err)
				}
			}
		})
	}
}
