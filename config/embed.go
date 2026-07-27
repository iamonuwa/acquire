// Package polldata embeds the poll table.
package polldata

import _ "embed"

// PollYAML is the embedded poll table (config/sources.poll.yaml).
//
//go:embed sources.poll.yaml
var PollYAML []byte
