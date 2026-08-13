// Package agentcontext exists for ONE reason: to embed templates/.
//
// go:embed cannot reach outside the embedding package's own directory, and the
// templates sit at the repo root next to the Python implementation that also
// installs them. Rather than keeping a second copy under go/ — two sources of
// truth for the same file is exactly the failure this whole tool is built to
// prevent — the module root became a package and the Go code imports it.
//
// This is why go.mod lives at the repo root while the Go sources live under
// go/: moving the module up costs nothing (the import paths are unchanged) and
// buys a single-file binary that carries its own scaffolding.
package agentcontext

import "embed"

// Templates is the scaffolding `ctx init` and `ctx bootstrap` write out:
// the manifest, the router blocks, the memory skeleton, and the hooks.
//
//go:embed templates
var Templates embed.FS
