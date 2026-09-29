package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sublink/models"
	"testing"
	"time"
)

func TestManualGeoIPDownloadCanBeStopped(t *testing.T) {
	setupSettingAPITestDB(t)
	geoIPDir := filepath.Join("..", ".agents", ".tmp")
	if err := os.MkdirAll(geoIPDir, 0750); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUBLINK_GEOIP_PATH", filepath.Join(geoIPDir, "geoip-stop-test.mmdb"))

	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	t.Cleanup(func() {
		server.CloseClientConnections()
		server.Close()
	})
	if err := models.SetSetting("geoip_download_url", server.URL); err != nil {
		t.Fatal(err)
	}

	response := performJSONRequest(t, DownloadGeoIP, http.MethodPost, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("start status = %d", response.Code)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("download request did not start")
	}

	response = performJSONRequest(t, StopGeoIPDownload, http.MethodPost, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("stop status = %d", response.Code)
	}
	deadline := time.After(3 * time.Second)
	for {
		downloading, _, errMsg, _ := GetGeoIPDownloadStatus()
		if !downloading {
			if !strings.Contains(errMsg, "下载已被用户停止") {
				t.Fatalf("download error = %q, want stopped", errMsg)
			}
			return
		}
		select {
		case <-deadline:
			t.Fatal("manual download did not stop")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
