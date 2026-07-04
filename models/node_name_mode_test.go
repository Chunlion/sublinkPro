package models

import "testing"

func TestNodeEffectiveNameUsesConfiguredMode(t *testing.T) {
	tests := []struct {
		name string
		node Node
		want string
	}{
		{
			name: "link mode uses original link name",
			node: Node{Name: "用户备注", LinkName: "上游名称", NameMode: NodeNameModeLink},
			want: "上游名称",
		},
		{
			name: "remark mode uses custom remark",
			node: Node{Name: "用户备注", LinkName: "上游名称", NameMode: NodeNameModeRemark},
			want: "用户备注",
		},
		{
			name: "remark mode falls back to link name when remark is empty",
			node: Node{Name: " ", LinkName: "上游名称", NameMode: NodeNameModeRemark},
			want: "上游名称",
		},
		{
			name: "invalid mode falls back to link mode",
			node: Node{Name: "用户备注", LinkName: "上游名称", NameMode: "unknown"},
			want: "上游名称",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.node.EffectiveName(); got != tt.want {
				t.Fatalf("EffectiveName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNodeNameSyncFromLinkName(t *testing.T) {
	tests := []struct {
		name        string
		node        Node
		newLinkName string
		want        string
	}{
		{
			name:        "link mode follows upstream rename",
			node:        Node{Name: "旧名称", LinkName: "旧名称", NameMode: NodeNameModeLink},
			newLinkName: "新名称",
			want:        "新名称",
		},
		{
			name:        "remark mode locks even when remark equals old upstream name",
			node:        Node{Name: "旧名称", LinkName: "旧名称", NameMode: NodeNameModeRemark},
			newLinkName: "新名称",
			want:        "旧名称",
		},
		{
			name:        "custom remark is preserved in remark mode",
			node:        Node{Name: "我的备注", LinkName: "旧名称", NameMode: NodeNameModeRemark},
			newLinkName: "新名称",
			want:        "我的备注",
		},
		{
			name:        "custom remark is preserved even in link mode",
			node:        Node{Name: "日本测试", LinkName: "日本 01", NameMode: NodeNameModeLink},
			newLinkName: "日本 02",
			want:        "日本测试",
		},
		{
			name:        "empty remark follows upstream name",
			node:        Node{Name: "", LinkName: "旧名称", NameMode: NodeNameModeLink},
			newLinkName: "新名称",
			want:        "新名称",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.node.NameAfterLinkNameUpdate(tt.newLinkName); got != tt.want {
				t.Fatalf("NameAfterLinkNameUpdate() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerateUniqueNodeNameUsesReservedNames(t *testing.T) {
	reserved := map[string]bool{"节点": true, "节点-2": true}

	got := GenerateUniqueNodeName("节点", 0, reserved)
	if got != "节点-3" {
		t.Fatalf("GenerateUniqueNodeName() = %q, want %q", got, "节点-3")
	}
	if !reserved["节点-3"] {
		t.Fatalf("GenerateUniqueNodeName() should reserve generated name")
	}
}

func TestGenerateUniqueNodeNameWithSourceUsesAtSuffix(t *testing.T) {
	reserved := map[string]bool{"香港 01": true}

	got := GenerateUniqueNodeNameWithSource("香港 01", "机场B", 0, reserved)
	if got != "香港 01@机场B" {
		t.Fatalf("GenerateUniqueNodeNameWithSource() = %q, want %q", got, "香港 01@机场B")
	}
}

func TestGenerateUniqueNodeNameWithSourceNumbersDuplicateSourceSuffix(t *testing.T) {
	reserved := map[string]bool{"香港 01": true, "香港 01@机场B": true}

	got := GenerateUniqueNodeNameWithSource("香港 01", "机场B", 0, reserved)
	if got != "香港 01@机场B-2" {
		t.Fatalf("GenerateUniqueNodeNameWithSource() = %q, want %q", got, "香港 01@机场B-2")
	}
}
