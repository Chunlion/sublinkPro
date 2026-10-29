package mihomo

import "testing"

func TestGetMihomoAdapterHY2PortHopping(t *testing.T) {
	for _, link := range []string{
		"hysteria2://password@192.0.2.1:24443-24543/?obfs=salamander&obfs-password=test-obfs#native",
		"hy2://password@[2001:db8::1]:24443-24543,25000#ipv6",
		"hy2://password@192.0.2.1:24443/?mport=24443-24543,25000#legacy",
	} {
		proxy, err := GetMihomoAdapter(link)
		if err != nil {
			t.Fatalf("create hopping adapter: %v", err)
		}
		if err := proxy.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
