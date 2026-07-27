// Package polldata embeds the poll table (SPEC §4.1) so the binary is
// self-contained (nothing extra to scp) and --dry-run needs no external file.
//
// The embedder lives beside the data because //go:embed cannot traverse "..".
// The poll-table types, loader, and validation live in internal/config; this
// package holds only the raw bytes.
package polldata

import _ "embed"

//go:embed sources.poll.yaml
var PollYAML []byte
