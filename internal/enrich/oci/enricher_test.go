package oci

import "testing"

func TestParseImageRef(t *testing.T) {
	parsed := ParseImageRef("registry.example.com/frontend:v2@sha256:deadbeef")
	if parsed.Repo != "registry.example.com/frontend" {
		t.Fatalf("unexpected repo: %q", parsed.Repo)
	}
	if parsed.Tag != "v2" {
		t.Fatalf("unexpected tag: %q", parsed.Tag)
	}
	if parsed.Digest != "sha256:deadbeef" {
		t.Fatalf("unexpected digest: %q", parsed.Digest)
	}
}
