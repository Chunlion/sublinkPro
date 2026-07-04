package main

import "testing"

func TestStaticCacheControlForPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "html is not cached", path: "index.html", want: "no-cache"},
		{name: "base path is not cached", path: "/", want: "no-cache"},
		{name: "service worker is not cached", path: "/sw.js", want: "no-cache"},
		{name: "manifest is not cached", path: "/manifest.webmanifest", want: "no-cache"},
		{name: "hashed assets are immutable", path: "/assets/index-abc123.js", want: "public, max-age=31536000, immutable"},
		{name: "workbox runtime is immutable", path: "/workbox-abc123.js", want: "public, max-age=31536000, immutable"},
		{name: "public images use short cache", path: "/images/favicon.svg", want: "public, max-age=86400"},
		{name: "root app icon uses short cache", path: "/icon-512.png", want: "public, max-age=86400"},
		{name: "api path has no cache override", path: "/api/v1/nodes/get", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := staticCacheControlForPath(tt.path); got != tt.want {
				t.Fatalf("staticCacheControlForPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
