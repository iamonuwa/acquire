---
name: add-source
description: How to add a new payer source to the cail-acquire poll table without breaking source discipline or the table-driven design. Use this skill whenever the user mentions adding a province, adding a payer, adding a jurisdiction, adding a source, wiring up Ontario or Alberta or BC or RAMQ or Quebec or Saskatchewan, resolving an `unresolved` source row, or editing `sources.yaml` or `sources.poll.yaml`, even if they describe it as a small config change. Also use it when a source URL appears to have moved or broken. Adding a source touches two repositories and has a verification protocol that is easy to skip, so consult this before making any edit.
---

# Adding a source to cail-acquire

Adding a source is the most error-prone routine operation in this repo. It spans two repositories, it is where unverified URLs enter the system, and it is the operation that tests whether the table-driven design still holds.

Work through the steps in order. Several of them can stop the process, and stopping is a valid outcome.

## Before anything: is this source in scope?

Adding a source is a human product decision, not a refactor. Confirm the user actually wants a new jurisdiction encoded, not just a URL fixed.

If they are fixing a URL on an existing source, skip to step 2 and then stop. A changed payload URL is normal operation for `url_kind: index` sources and usually means the manifest is fine and the anchor pattern needs adjusting.

## Step 1: Verify the source is real and harvestable

Find the publisher's own live page that indexes the criteria document. Then confirm the payload link is actually present on that page.

The rule is that no URL enters `sources.yaml` unless it was observed as a link on the publisher's own live page. A URL that looks right, follows the pattern other provinces use, or appears in a third-party document does not qualify. This rule exists because a plausible URL that 404s six months later fails silently, and a plausible URL that resolves to a *withdrawn* document fails worse: it produces confident wrong criteria.

Stop and report if any of these are true:

- The payload is behind authentication.
- The index page is a JavaScript application that renders links client-side with no public data endpoint.
- You cannot find the payload link on a page the publisher controls.

The precedent is NLPDP's Provider Notifications, which redirects to an authenticated Angular application. Bulletin acquisition was ruled out of scope rather than scraped. Do not build a fetcher against an authenticated single-page application.

If the source cannot be verified, add it to the poll table as a `state: unresolved` row carrying no URL, and stop. CI counts unresolved rows, so the gap stays visible. An absent source is invisible; an unresolved one is a tracked open item.

## Step 2: Determine URL stability

This decides the fetch strategy and it is the step most often gotten wrong.

Look at the payload filename. If it embeds a date, a version, an edition number, or anything else that will change on revision, the URL is **not stable**.

- Not stable, which is the common case: set `url_kind: index`, point `url` at the index page, and use `scrape_anchor`. The payload URL is discovered per run.
- Genuinely stable: set `url_kind: payload`. Be skeptical. Verify across at least two known revisions before claiming stability.

Never hardcode an observed payload URL anywhere in the codebase. NLPDP publishes `Criteria-<Month>-<Year>.pdf`; a fetcher pointed at the July file breaks silently in August, and silently is the problem. The observed payload URL goes in `last_payload_url` as provenance, which the fetcher never reads as input.

Ontario has a further wrinkle recorded in the decision record: its edition page has linked a withdrawn file while a live release sat elsewhere. Ontario needs `probe_forward`, which is deferred and not built. If the user is adding Ontario, that strategy has to be built first and its filename pattern derived from live observation, never from memory.

## Step 3: Record the licence

Reuse terms are per-source data, not institutional memory. Find the publisher's terms page and record both `licence` and `licence_ref`.

Do not assume a province with an open data portal releases its health program documents under that licence. Newfoundland operates an Open Government Licence that covers `opendata.gov.nl.ca` datasets only; its Health and Community Services criteria PDFs are all rights reserved. Getting this wrong creates a redistribution problem that surfaces years later at productization.

If the terms are ambiguous, record `licence: UNRESOLVED` with the reference URL and flag it. Do not guess.

## Step 4: Write the manifest entry

In `cail-rules/sources.yaml`. All fields:

```yaml
- id: <jurisdiction>-<short-name>
  jurisdiction: <two-letter code>
  publisher: <publishing body>
  url: <index page or stable payload URL>
  url_kind: index | payload
  url_observed_on: <URL of the page where you saw the link>
  url_observed_date: <YYYY-MM-DD>
  payload_confirmed: false
  last_payload_url: <what the anchor resolved to, provenance only>
  licence: <identifier>
  licence_ref: <terms page URL>
  raw_hash: UNVERIFIED
  normalized_hash: UNVERIFIED
  state: active
```

`url_observed_on` and `payload_confirmed` are deliberately separate. The first says you saw a link; the second says bytes were actually retrieved. Conflating them lets an unfetched source look verified.

Leave `payload_confirmed: false` and both hashes `UNVERIFIED`. They flip on the first successful fetch. Writing a plausible hash to make CI green defeats the entire mechanism.

## Step 5: Add the poll row

In `cail-acquire/config/sources.poll.yaml`:

```yaml
  - id: <same id as the manifest entry>
    jurisdiction: <two-letter code>
    publisher: <publishing body>
    fetch: scrape_anchor | archive
    normalize: pdf | zip_members
    poll_interval: <duration>
    staleness_alarm_days: <int>
    enabled: false
    scrape:
      link_scope: '<CSS selector>'
      anchor_pattern: '<regex anchored with $>'
    strip_rules: []
```

Start `enabled: false`. Turn it on after the fetch succeeds by hand.

Start `strip_rules` empty. Strip rules are written against observed churn during the soak, never invented ahead of it. A rule invented in advance is how a real criteria change gets normalized away and never opens a merge request.

The `id` must match the manifest entry exactly. Config load treats a mismatch as fatal, which is the point.

## Step 6: Reuse a fetch strategy and normalizer if you possibly can

Fetch strategies are an enum, not per-source code. Two exist: `scrape_anchor` and `archive`. Normalizers: `pdf` and `zip_members`.

Try hard to reuse. Most provincial criteria are a PDF behind an index page, which is exactly `scrape_anchor` plus `pdf`.

Write a new one only if the source genuinely does not fit. If you do, it goes in `internal/fetch/` or `internal/normalize/` as one new file, added to the enum. Per-source branching inside an existing strategy is the failure mode to avoid: it turns config into code and the next source becomes harder rather than easier.

## Step 7: PHI patterns for the jurisdiction

Each province has its own health card number format. The gate needs the new one before the source goes live.

Find the published format specification from the province itself. Do not infer a format from an example number.

If you cannot find it, ship the source with `gate_partial: true` in the status file for that jurisdiction so the gap is visible rather than assumed closed. Visible-and-incomplete beats invisible-and-assumed-complete.

The gate is tuned to over-fire on purpose. A false positive costs one alert. A false negative puts patient data in a git object, which cannot be undone in practice. Do not tune a new pattern toward precision.

Add synthetic fixtures for the new pattern. Never commit a real positive to `testdata/`.

## Step 8: Golden fixture

Save a copy of the index page HTML to `testdata/` and write a `scrape_anchor` test against it covering three cases: the happy path, zero matches, and two matches.

Zero and two are both failures. Two matches must never degrade to picking the first. A source that silently picks the wrong one of two documents is worse than a source that fails, because the failure is loud and the wrong pick is not.

## Step 9: Run the architectural test

```bash
make check-source-boundary
# or directly:
./scripts/architectural_test.sh origin/main
```

The script lives in the repo, not in this skill, because CI runs it on every merge request touching `config/sources.poll.yaml`. A boundary nothing tests is not a boundary.

Adding a source should require a config row, at most one new fetch strategy, at most one new normalizer, and fixtures. Nothing else.

If the script reports changes to `internal/store`, `internal/vcs`, `internal/diff`, `internal/config`, or `cmd/`, the table-driven design has failed. That is a real finding and worth surfacing immediately rather than working around. Report it to the user; do not quietly make the change fit.

## Step 10: First fetch, by hand

```bash
cail-acquire poll --source=<id> --dry-run --force
```

Confirm the anchor resolves, the payload downloads, normalization produces sensible text, and the hash is stable across two consecutive runs.

Then flip `payload_confirmed: true` and write the real hashes to the manifest. Only now set `enabled: true`.

## What this skill does not cover

Encoding the source's criteria into rules. That happens in `cail-rules` by a human and is out of scope for this repository.