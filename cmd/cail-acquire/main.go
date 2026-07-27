// Command cail-acquire polls a fixed table of Canadian payer source URLs.
//
// Milestone 2 implements only the `poll` subcommand's resolve-and-report path:
// discover each source's current payload URL via scrape_anchor and report it.
// No payload fetch, hash, store, PHI gate, diff, or git — those are later
// milestones (SPEC §10). The other subcommands are reserved, not yet built.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	polldata "gitlab.com/cail-health/cail-acquire/config"
	"gitlab.com/cail-health/cail-acquire/internal/config"
	"gitlab.com/cail-health/cail-acquire/internal/fetch"
	"gitlab.com/cail-health/cail-acquire/internal/manifest"
)

// Exit codes (SPEC §5.1), aggregate across sources, highest severity wins.
const (
	exitOK     = 0  // all sources checked, no change
	exitChange = 10 // change found, MR opened — normal operation (not reachable in M2)
	exitFetch  = 20 // fetch, normalize, or store failure
	exitPHI    = 30 // PHI gate fired (not reachable in M2)
	exitConfig = 40 // config or manifest error, nothing ran
)

// defaultManifestPath is the cail-rules working copy on the droplet (SPEC §9.2).
// Override with --manifest for local development.
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
		fmt.Fprintf(os.Stderr, "cail-acquire: %q is not implemented yet (later milestone)\n", args[0])
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
	force := fs.Bool("force", false, "ignore poll_interval (no effect in milestone 2; interval state is milestone 10)")
	configPath := fs.String("config", "", "path to sources.poll.yaml (default: embedded table)")
	manifestPath := fs.String("manifest", defaultManifestPath, "path to cail-rules sources.yaml")
	if err := fs.Parse(args); err != nil {
		return exitConfig
	}
	_ = force // accepted for forward-compatibility; interval skipping arrives at M10

	// Load and validate the poll table (SPEC §4.1). Any failure is fatal (40).
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

	// Load the manifest and run the §4.1 cross-repo validation.
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
		if code := pollSource(ctx, client, man, src, *dryRun); code > exit {
			exit = code
		}
	}
	if *source != "" && polled == 0 {
		fmt.Fprintf(os.Stderr, "poll: no enabled source matched --source=%q\n", *source)
		return exitConfig
	}
	return exit
}

// pollSource resolves one source's payload URL and reports it. It returns the
// severity for this source (exitOK or exitFetch).
func pollSource(ctx context.Context, client *fetch.Client, man *manifest.Manifest, src *config.Source, dryRun bool) int {
	indexURL, err := man.IndexURL(src.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] %v\n", src.ID, err)
		return exitFetch
	}
	strat, err := fetch.Get(src.Fetch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] %v\n", src.ID, err)
		return exitFetch
	}
	m, err := strat.Resolve(ctx, client, indexURL, fetch.SpecFor(src))
	if err != nil {
		// Zero-match and multi-match are loud, typed failures (SPEC §6.1).
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

	prefix := ""
	if dryRun {
		prefix = "[dry-run] "
	}
	fmt.Printf("%s[%s] resolved payload: %s\n", prefix, src.ID, m.URL)
	if m.AnchorText != "" {
		fmt.Printf("            anchor: %s\n", m.AnchorText)
	}
	return exitOK
}
