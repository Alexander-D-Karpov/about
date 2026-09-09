// Package build carries values stamped into the binary at link time.
package buildinfo

// Env names the deployment a binary was built for. It is empty for a plain
// `go build` and set by the Makefile's preview target:
//
//	-ldflags "-X github.com/Alexander-D-Karpov/about/internal/buildinfo.Env=preview"
//
// Nothing reads it from the environment on purpose - preview and prod run the
// same config on the same host, so only the binary itself can tell them apart.
var Env string

// IsPreview reports whether this binary was built for the preview deployment.
// Preview pages carry a badge so a stale preview tab is never mistaken for the
// live site.
func IsPreview() bool { return Env == "preview" }
