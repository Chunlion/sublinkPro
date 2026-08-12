package monitor

import "testing"

func TestFormatBytesUsesNumericText(t *testing.T) {
	tests := []struct {
		bytes uint64
		want  string
	}{
		{bytes: 512, want: "512 B"},
		{bytes: 1536, want: "1.5 KB"},
	}

	for _, tt := range tests {
		if got := FormatBytes(tt.bytes); got != tt.want {
			t.Fatalf("FormatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}

func TestFormatDurationUsesNumericText(t *testing.T) {
	if got := FormatDuration(90061); got != "1天1时1分" {
		t.Fatalf("FormatDuration(90061) = %q, want %q", got, "1天1时1分")
	}
	if got := FormatDuration(61); got != "1分1秒" {
		t.Fatalf("FormatDuration(61) = %q, want %q", got, "1分1秒")
	}
}
