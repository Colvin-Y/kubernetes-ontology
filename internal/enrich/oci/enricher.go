package oci

import "strings"

type ImageRef struct {
	Original string
	Repo     string
	Tag      string
	Digest   string
}

func ParseImageRef(ref string) ImageRef {
	out := ImageRef{Original: ref}
	remaining := ref
	if parts := strings.SplitN(ref, "@", 2); len(parts) == 2 {
		remaining = parts[0]
		out.Digest = parts[1]
	}
	if idx := strings.LastIndex(remaining, ":"); idx != -1 && !strings.Contains(remaining[idx+1:], "/") {
		out.Repo = remaining[:idx]
		out.Tag = remaining[idx+1:]
	} else {
		out.Repo = remaining
	}
	return out
}
