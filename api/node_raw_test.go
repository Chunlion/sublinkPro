package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"

	"sublink/database"
	"sublink/internal/testutil"
	"sublink/models"
	"sublink/node/protocol"
)

func setupNodeRawAPITestDB(t *testing.T) {
	t.Helper()

	oldDB := database.DB
	oldDialect := database.Dialect
	oldInitialized := database.IsInitialized

	db := testutil.OpenMemoryDB(t, "node_raw_api_test")
	if err := db.AutoMigrate(&models.Node{}); err != nil {
		t.Fatalf("auto migrate nodes: %v", err)
	}

	database.DB = db
	database.Dialect = database.DialectSQLite
	database.IsInitialized = false
	if err := models.InitNodeCache(); err != nil {
		t.Fatalf("init node cache: %v", err)
	}

	t.Cleanup(func() {
		database.DB = oldDB
		database.Dialect = oldDialect
		database.IsInitialized = oldInitialized
		if oldDB != nil {
			_ = models.InitNodeCache()
		}
		testutil.CloseDB(t, db)
	})
}

func createNodeRawAPITestNode(t *testing.T, node models.Node) models.Node {
	t.Helper()

	if node.Protocol == "" {
		node.Protocol = "http"
	}
	node.LinkHash = nodeRawAPITestLinkHash(node.Link)
	if err := database.DB.Create(&node).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := models.InitNodeCache(); err != nil {
		t.Fatalf("refresh node cache: %v", err)
	}
	return node
}

func nodeRawAPITestLinkHash(link string) string {
	sum := sha256.Sum256([]byte(link))
	return hex.EncodeToString(sum[:])
}

func nodeRawAPITestLink(name string) string {
	return protocol.EncodeHTTPURL(protocol.HTTP{
		Name:     name,
		Server:   "node-raw-api.example",
		Port:     8080,
		Username: "user",
		Password: "pass",
	})
}

func assertNodeRawAPIUpdatedNode(t *testing.T, nodeID int, wantName string, wantLinkName string) {
	t.Helper()

	var stored models.Node
	if err := database.DB.First(&stored, nodeID).Error; err != nil {
		t.Fatalf("reload updated node: %v", err)
	}
	if stored.Name != wantName {
		t.Fatalf("stored Name = %q, want %q", stored.Name, wantName)
	}
	if stored.LinkName != wantLinkName {
		t.Fatalf("stored LinkName = %q, want %q", stored.LinkName, wantLinkName)
	}

	cached, ok := models.GetNodeByID(nodeID)
	if !ok {
		t.Fatalf("updated node missing from cache")
	}
	if cached.Name != wantName {
		t.Fatalf("cached Name = %q, want %q", cached.Name, wantName)
	}
	if cached.LinkName != wantLinkName {
		t.Fatalf("cached LinkName = %q, want %q", cached.LinkName, wantLinkName)
	}
}

func TestUpdateNodeRawInfoPreservesCustomRemarkWhenLinkNameChanges(t *testing.T) {
	setupNodeRawAPITestDB(t)

	node := createNodeRawAPITestNode(t, models.Node{
		Name:     "custom-remark",
		LinkName: "old-link-name",
		NameMode: models.NodeNameModeLink,
		Link:     nodeRawAPITestLink("old-link-name"),
		Source:   "manual",
	})

	recorder := performJSONRequest(t, UpdateNodeRawInfo, http.MethodPost, UpdateNodeRawRequest{
		NodeID: node.ID,
		Fields: map[string]any{
			"Name": "new-link-name",
		},
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	response := decodeAPIResponse(t, recorder)
	if response.Code != 200 {
		t.Fatalf("response code = %d, msg = %s", response.Code, response.Msg)
	}

	assertNodeRawAPIUpdatedNode(t, node.ID, "custom-remark", "new-link-name")
}

func TestUpdateNodeRawInfoSyncsRemarkWhenUnsetOrDefault(t *testing.T) {
	tests := []struct {
		name        string
		nodeName    string
		wantName    string
		linkName    string
		newLinkName string
	}{
		{
			name:        "empty remark",
			nodeName:    "",
			wantName:    "new-empty-link-name",
			linkName:    "old-empty-link-name",
			newLinkName: "new-empty-link-name",
		},
		{
			name:        "default remark",
			nodeName:    "old-default-link-name",
			wantName:    "new-default-link-name",
			linkName:    "old-default-link-name",
			newLinkName: "new-default-link-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupNodeRawAPITestDB(t)

			node := createNodeRawAPITestNode(t, models.Node{
				Name:     tt.nodeName,
				LinkName: tt.linkName,
				NameMode: models.NodeNameModeLink,
				Link:     nodeRawAPITestLink(tt.linkName),
				Source:   "manual",
			})

			recorder := performJSONRequest(t, UpdateNodeRawInfo, http.MethodPost, UpdateNodeRawRequest{
				NodeID: node.ID,
				Fields: map[string]any{
					"Name": tt.newLinkName,
				},
			})
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			response := decodeAPIResponse(t, recorder)
			if response.Code != 200 {
				t.Fatalf("response code = %d, msg = %s", response.Code, response.Msg)
			}

			assertNodeRawAPIUpdatedNode(t, node.ID, tt.wantName, tt.newLinkName)
		})
	}
}
