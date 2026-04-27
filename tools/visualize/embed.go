package visualize

import _ "embed"

// IndexHTML is the dependency-free viewer UI used by the Go viewer binary.
//
//go:embed index.html
var IndexHTML string
