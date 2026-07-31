// Command cail-acquire polls a fixed table of Canadian payer source URLs.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	polldata "gitlab.com/cail-health/cail-acquire/config"
	"gitlab.com/cail-health/cail-acquire/internal/catalogue"
	"gitlab.com/cail-health/cail-acquire/internal/fetch"
	"gitlab.com/cail-health/cail-acquire/internal/manifest"
	"gitlab.com/cail-health/cail-acquire/internal/normalize"
	"gitlab.com/cail-health/cail-acquire/internal/payloadstore"
	"gitlab.com/cail-health/cail-acquire/internal/phigate"
	"gitlab.com/cail-health/cail-acquire/internal/polltable"
	"gitlab.com/cail-health/cail-acquire/internal/runlog"
	"gitlab.com/cail-health/cail-acquire/internal/statusfile"
	"gitlab.com/cail-health/cail-acquire/internal/verifydin"
)

// pollResult is what pollSource observed, for the status file.
type pollResult struct {
	changed        bool
	lastPayloadURL string
	normalizedHash string
}

// Exit codes, aggregated across sources with highest severity winning.
const (
	exitOK          = 0
	exitChange      = 10
	exitFetch       = 20
	exitPHI         = 30
	exitConfig      = 40
	exitFingerprint = 50
)

// defaultManifestPath is the droplet's cail-rules working copy.
const defaultManifestPath = "/var/lib/cail-acquire/cail-rules/sources.yaml"

// defaultRunlogPath is the append-only run log on the droplet.
const defaultRunlogPath = "/var/lib/cail-acquire/runlog.jsonl"

func main() { os.Exit(dispatch(os.Args[1:])) }

func dispatch(args []string) int {
	if len(args) == 0 {
		usage()
		return exitConfig
	}
	switch args[0] {
	case "poll":
		return runPoll(args[1:])
	case "catalogue":
		return runCatalogue(args[1:])
	case "verify-din":
		return runVerifyDIN(args[1:])
	case "status":
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
  catalogue    build the DPD catalogue from an extract (--input allfiles.zip)
  status       (not implemented) build and publish the status file
  verify-din   resolve a single DIN via the catalogue, then the DPD API (--din)

run "cail-acquire poll -h" for poll flags.
`)
}

func runPoll(args []string) int {
	fs := flag.NewFlagSet("poll", flag.ContinueOnError)
	source := fs.String("source", "", "only poll this source id (default: all enabled)")
	dryRun := fs.Bool("dry-run", false, "resolve and report without writing anywhere")
	fs.Bool("force", false, "ignore poll_interval (not yet enforced)")
	configPath := fs.String("config", "", "path to sources.poll.yaml (default: embedded table)")
	manifestPath := fs.String("manifest", defaultManifestPath, "path to cail-rules sources.yaml")
	runlogPath := fs.String("runlog", defaultRunlogPath, "path to the run log")
	if err := fs.Parse(args); err != nil {
		return exitConfig
	}

	raw := polldata.PollYAML
	if *configPath != "" {
		b, err := os.ReadFile(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "config: %v\n", err)
			return exitConfig
		}
		raw = b
	}
	table, err := polltable.Load(raw)
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

	// Assert R2 secrets and build the store before any network call.
	// --dry-run never writes, so it needs no credentials.
	var st payloadstore.Store
	if !*dryRun {
		if err := payloadstore.RequireSecrets(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitConfig
		}
		r2, err := payloadstore.NewR2(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitConfig
		}
		st = r2
	}

	exit := exitOK
	polled := 0
	var inputs []statusfile.Input
	for i := range table.Sources {
		src := &table.Sources[i]
		if src.IsUnresolved() || !src.Enabled {
			continue
		}
		if *source != "" && src.ID != *source {
			continue
		}
		polled++
		var res pollResult
		code := pollSource(ctx, client, man, st, *manifestPath, *runlogPath, src, *dryRun, &res)
		if code > exit {
			exit = code
		}
		url := res.lastPayloadURL
		if url == "" {
			if e, ok := man.Entry(src.ID); ok {
				url = e.LastPayloadURL
			}
		}
		inputs = append(inputs, statusfile.Input{
			ID:                 src.ID,
			Jurisdiction:       src.Jurisdiction,
			Failed:             code != exitOK && code != exitChange,
			Changed:            res.changed,
			LastPayloadURL:     url,
			NormalizedHash:     res.normalizedHash,
			StalenessAlarmDays: src.StalenessAlarmDays,
		})
	}
	if *source != "" && polled == 0 {
		fmt.Fprintf(os.Stderr, "poll: no enabled source matched --source=%q\n", *source)
		return exitConfig
	}
	// Publish status after all sources, even after failures — an invisible
	// failure is worse than a visible one.
	if !*dryRun && st != nil {
		publishStatus(ctx, st, inputs)
	}
	return exit
}

func publishStatus(ctx context.Context, st payloadstore.Store, inputs []statusfile.Input) {
	var prev *statusfile.Status
	if b, found, err := st.Get(ctx, payloadstore.StatusKey); err != nil {
		fmt.Fprintf(os.Stderr, "status: read previous: %v\n", err)
	} else if found {
		prev, _ = statusfile.Parse(b)
	}
	body, err := statusfile.Marshal(statusfile.Build(time.Now(), inputs, prev))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	if err := st.Put(ctx, payloadstore.StatusKey, body); err != nil {
		fmt.Fprintf(os.Stderr, "status: publish: %v\n", err)
	}
}

func pollSource(ctx context.Context, client *fetch.Client, man *manifest.Manifest, st payloadstore.Store, manifestPath, runlogPath string, src *polltable.Source, dryRun bool, res *pollResult) int {
	indexURL, err := man.IndexURL(src.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] %v\n", src.ID, err)
		return exitFetch
	}
	if src.Normalize == polltable.NormalizePDF {
		if err := normalize.AssertAvailable(); err != nil {
			fmt.Fprintf(os.Stderr, "[%s] %v\n", src.ID, err)
			return exitFetch
		}
	}

	strat, err := fetch.StrategyFor(src.Fetch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] %v\n", src.ID, err)
		return exitFetch
	}
	m, err := strat.Resolve(ctx, client, indexURL, fetch.SpecFor(src))
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] %v\n", src.ID, err)
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

	fp, err := normalize.Fingerprint(ctx, src.Normalize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] %v\n", src.ID, err)
		return exitFetch
	}

	rawHash := sha256Hex(resp.Body)
	normHash := sha256Hex(text)
	entry, _ := man.Entry(src.ID)

	// A poppler change makes every hash in the run untrustworthy, so a
	// fingerprint mismatch halts before any write and is never a content change.
	if entry != nil && isBaselined(entry.NormalizerFingerprint) && entry.NormalizerFingerprint != fp {
		fmt.Fprintf(os.Stderr, "[%s] normalizer fingerprint mismatch: manifest=%q observed=%q\n",
			src.ID, entry.NormalizerFingerprint, fp)
		return exitFingerprint
	}

	changed := entry == nil || entry.NormalizedHash != normHash
	res.changed = changed
	res.lastPayloadURL = m.URL.String()
	res.normalizedHash = normHash

	prefix := ""
	if dryRun {
		prefix = "[dry-run] "
	}
	fmt.Printf("%s[%s] payload %s (%d bytes)\n", prefix, src.ID, m.URL, resp.ByteCount)
	fmt.Printf("            normalized_hash %s change=%t fingerprint=%s\n", normHash, changed, fp)
	if resp.LastModified != "" || resp.ETag != "" {
		fmt.Printf("            last-modified=%q etag=%q\n", resp.LastModified, resp.ETag)
	}

	// Runlog records every run, including no-change days. --dry-run writes nowhere.
	if !dryRun {
		rec := runlog.Record{
			TS:                    time.Now().UTC().Format(time.RFC3339),
			SourceID:              src.ID,
			HTTPStatus:            resp.StatusCode,
			FinalURL:              resp.FinalURL.String(),
			Bytes:                 resp.ByteCount,
			LastModified:          resp.LastModified,
			ETag:                  resp.ETag,
			RawHash:               "sha256:" + rawHash,
			NormalizedHash:        "sha256:" + normHash,
			NormalizerFingerprint: fp,
			Changed:               changed,
		}
		if err := runlog.Append(runlogPath, rec); err != nil {
			fmt.Fprintf(os.Stderr, "[%s] %v\n", src.ID, err)
			return exitFetch
		}
		if st != nil {
			if lines, lerr := runlog.SourceLines(runlogPath, src.ID); lerr == nil {
				if perr := st.Put(ctx, "runlog/"+src.ID+".jsonl", lines); perr != nil {
					fmt.Fprintf(os.Stderr, "[%s] runlog mirror failed: %v\n", src.ID, perr)
				}
			}
		}
	}

	if !changed {
		return exitOK
	}

	// PHI gate before any write.
	if hit := phigate.Check(text); hit.Hit {
		fmt.Fprintf(os.Stderr, "[%s] PHI gate fired: %s\n", src.ID, hit.Reason)
		return exitPHI
	}

	if dryRun {
		return exitOK
	}

	// R2 first: content-addressed orphans are harmless. Two objects.
	if err := st.Put(ctx, payloadstore.RawKey(src.ID, rawHash), resp.Body); err != nil {
		fmt.Fprintf(os.Stderr, "[%s] %v\n", src.ID, err)
		return exitFetch
	}
	if err := st.Put(ctx, payloadstore.NormKey(src.ID, normHash), text); err != nil {
		fmt.Fprintf(os.Stderr, "[%s] %v\n", src.ID, err)
		return exitFetch
	}

	confirmed := true
	urlStr := m.URL.String()
	if err := manifest.UpdateEntry(manifestPath, src.ID, manifest.Update{
		PayloadConfirmed:      &confirmed,
		RawHash:               &rawHash,
		NormalizedHash:        &normHash,
		NormalizerFingerprint: &fp,
		LastPayloadURL:        &urlStr,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "[%s] %v\n", src.ID, err)
		return exitFetch
	}
	return exitOK
}

func isBaselined(fp string) bool {
	return fp != "" && fp != "UNVERIFIED"
}

func runCatalogue(args []string) int {
	fs := flag.NewFlagSet("catalogue", flag.ContinueOnError)
	input := fs.String("input", "", "path to allfiles.zip")
	out := fs.String("out", "catalogue", "output directory")
	if err := fs.Parse(args); err != nil {
		return exitConfig
	}
	if *input == "" {
		fmt.Fprintln(os.Stderr, "catalogue: --input <allfiles.zip> is required")
		return exitConfig
	}
	data, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "catalogue: %v\n", err)
		return exitConfig
	}
	meta, err := catalogue.Build(data, *out)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitFetch
	}
	fmt.Printf("catalogue: %d ingredients, %d DINs -> %s (%s)\n",
		meta.IngredientCount, meta.DINCount, *out, meta.SourceHash)
	return exitOK
}

func runVerifyDIN(args []string) int {
	fs := flag.NewFlagSet("verify-din", flag.ContinueOnError)
	din := fs.String("din", "", "DIN to verify")
	catDir := fs.String("catalogue", "", "catalogue directory holding din-index.json")
	if err := fs.Parse(args); err != nil {
		return exitConfig
	}
	if *din == "" {
		fmt.Fprintln(os.Stderr, "verify-din: --din <DIN> is required")
		return exitConfig
	}

	dinIndex := map[string][]string{}
	if *catDir != "" {
		b, err := os.ReadFile(filepath.Join(*catDir, "din-index.json"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "verify-din: %v\n", err)
			return exitConfig
		}
		if err := json.Unmarshal(b, &dinIndex); err != nil {
			fmt.Fprintf(os.Stderr, "verify-din: parse din-index: %v\n", err)
			return exitConfig
		}
	}

	res, err := verifydin.Verify(context.Background(), fetch.NewClient(), verifydin.DefaultAPIBase, *din, dinIndex)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitFetch
	}
	switch res.Status {
	case verifydin.InCatalogue:
		fmt.Printf("%s: in catalogue -> %v\n", res.DIN, res.Slugs)
	case verifydin.ExtractLagged:
		fmt.Printf("%s: EXTRACT_LAGGED (in DPD API as %q, absent from the catalogue)\n", res.DIN, res.Brand)
	case verifydin.NotFound:
		fmt.Printf("%s: not found in the catalogue or the DPD API\n", res.DIN)
	}
	return exitOK
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
