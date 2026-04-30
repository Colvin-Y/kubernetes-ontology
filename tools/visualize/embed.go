package visualize

import _ "embed"

// IndexHTML is the viewer UI used by the Go viewer binary.
//
//go:embed index.html
var IndexHTML string

// CytoscapeJS is the vendored professional graph renderer used by the viewer.
//
//go:embed vendor/cytoscape.min.js
var CytoscapeJS string
