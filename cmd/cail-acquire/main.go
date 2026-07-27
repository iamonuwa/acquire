// Command cail-acquire polls a fixed table of Canadian payer source URLs.
// Milestone 2 implements only poll's resolve-and-report path.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"

	polldata "gitlab.com/cail-health/cail-acquire/config"
	"gitlab.com/cail-health/cail-acquire/internal/config"
	"gitlab.com/cail-health/cail-acquire/internal/fetch"
	"gitlab.com/cail-health/cail-acquire/internal/manifest"
	"gitlab.com/cail-health/cail-acquire/internal/normalize"
)

// Exit codes, aggregated across sources with highest severity winning.
const (
	exitOK     = 0
	exitChange = 10
	exitFetch  = 20
	exitPHI    = 30
	exitConfig = 40
)

// defaultManifestPath is the droplet's cail-rules working copy.
const defaultManifestPath = "/var/lib/cail-acquire/cail-rules/sources.yaml"

func main() { os.Exit(dispatch(os.Args[1:])) }

func dispatch(args []string) int {
	if len(args) == 0 {
		usage()
		return exitConfig
	}
	switch args[0] {
	case "poll":
		return runPoll(args[1:])
	case "catalogue", "status", "verify-din":
		fmt.Fprintf(os.Stderr, "cail-acquire: %q is not implemented yet\n", args[0])
		return exitConfig
	default:
		fmt.Fprintf(os.Stderr, "cail-acquire: unknown subcommand %q\n", args[0])
		usage()
		return exitConfig
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `cail-acquire — CAIL source acquisition

usage: cail-acquire <subcommand> [flags]

subcommands:
  poll         resolve source payload URLs and report changes
  catalogue    (not implemented) build the DPD catalogue
  status       (not implemented) build and publish the status file
  verify-din   (not implemented) single-DIN DPD API fallback

run "cail-acquire poll -h" for poll flags.
`)
}

func runPoll(args []string) int {
	fs := flag.NewFlagSet("poll", flag.ContinueOnError)
	source := fs.String("source", "", "only poll this source id (default: all enabled)")
	dryRun := fs.Bool("dry-run", false, "resolve and report without writing anywhere")
	force := fs.Bool("force", false, "ignore poll_interval (no effect until milestone 10)")
	configPath := fs.String("config", "", "path to sources.poll.yaml (default: embedded table)")
	manifestPath := fs.String("manifest", defaultManifestPath, "path to cail-rules sources.yaml")
	if err := fs.Parse(args); err != nil {
		return exitConfig
	}
	_ = force

	raw := polldata.PollYAML
	if *configPath != "" {
		b, err := os.ReadFile(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "config: %v\n", err)
			return exitConfig
		}
		raw = b
	}
	table, err := config.Load(raw)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitConfig
	}

	man, err := manifest.Load(*manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitConfig
	}
	if err := table.ValidateAgainstManifest(man); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitConfig
	}

	client := fetch.NewClient()
	ctx := context.Background()

	exit := exitOK
	polled := 0
	for i := range table.Sources {
		src := &table.Sources[i]
		if src.IsUnresolved() || !src.Enabled {
			continue
		}
		if *source != "" && src.ID != *source {
			continue
		}
		polled++
		if code := pollSource(ctx, client, man, *manifestPath, src, *dryRun); code > exit {
			exit = code
		}
	}
	if *source != "" && polled == 0 {
		fmt.Fprintf(os.Stderr, "poll: no enabled source matched --source=%q\n", *source)
		return exitConfig
	}
	return exit
}

func pollSource(ctx context.Context, client *fetch.Client, man *manifest.Manifest, manifestPath string, src *config.Source, dryRun bool) int {
	indexURL, err := man.IndexURL(src.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] %v\n", src.ID, err)
		return exitFetch
	}
	if src.Normalize == config.NormalizePDF {
		if err := normalize.AssertAvailable(); err != nil {
			fmt.Fprintf(os.Stderr, "[%s] %v\n", src.ID, err)
			return exitFetch
		}
	}

	strat, err := fetch.Get(src.Fetch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] %v\n", src.ID, err)
		return exitFetch
	}
	m, err := strat.Resolve(ctx, client, indexURL, fetch.SpecFor(src))
	if err != nil {
		var nm *fetch.NoMatchError
		var mm *fetch.MultiMatchError
		switch {
		case errors.As(err, &nm), errors.As(err, &mm):
			fmt.Fprintf(os.Stderr, "[%s] %v\n", src.ID, err)
		default:
			fmt.Fprintf(os.Stderr, "[%s] fetch failed: %v\n", src.ID, err)
		}
		return exitFetch
	}

	resp, err := client.Fetch(ctx, m.URL.String())
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] payload fetch failed: %v\n", src.ID, err)
		return exitFetch
	}
	text, err := normalize.Apply(ctx, src.Normalize, resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] %v\n", src.ID, err)
		return exitFetch
	}

	rawHash := sha256Hex(resp.Body)
	normHash := sha256Hex(text)

	entry, _ := man.Entry(src.ID)
	changed := entry == nil || entry.NormalizedHash != normHash

	prefix := ""
	if dryRun {
		prefix = "[dry-run] "
	}
	fmt.Printf("%s[%s] payload %s (%d bytes)\n", prefix, src.ID, m.URL, resp.ByteCount)
	fmt.Printf("            normalized_hash %s change=%t\n", normHash, changed)
	if resp.LastModified != "" || resp.ETag != "" {
		fmt.Printf("            last-modified=%q etag=%q\n", resp.LastModified, resp.ETag)
	}

	// M3 persists provenance only — no R2, no git.
	if dryRun {
		return exitOK
	}
	confirmed := true
	urlStr := m.URL.String()
	if err := manifest.UpdateEntry(manifestPath, src.ID, manifest.Update{
		PayloadConfirmed: &confirmed,
		RawHash:          &rawHash,
		NormalizedHash:   &normHash,
		LastPayloadURL:   &urlStr,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "[%s] %v\n", src.ID, err)
		return exitFetch
	}
	return exitOK
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
