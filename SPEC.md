# SPEC.md

Build specification for `cail-acquire`. Scope: two sources, NLPDP first, Health Canada DPD second.

**Authority.** `DECISIONS.md` in `cail-rules` is the locked decision record. This file is the build document that implements it. Where the two disagree, `DECISIONS.md` wins and this file is wrong. Section citations below refer to `DECISIONS.md` unless stated otherwise.

**Status as of 2026-07-27.** All corrections identified in the source verification pass are now applied to `DECISIONS.md` (NLPDP host, DPD extract date, DPD product-role/biosimilar mechanism) and committed to `cail-rules`. This file contains no outstanding corrections. Milestone 1 is complete.

Confidence markers: VERIFIED means observed on the publisher's own live page 2026-07-27. INFERENCE means reasoned, with the reasoning stated. UNVERIFIED means not confirmed, and no value has been substituted.

---

## 1. Why NL first

NLPDP is one PDF behind one anchor. It exercises the entire pipeline end to end in a day: two-hop fetch, `pdftotext -layout`, normalization, hashing, PHI gate, R2 put, semantic diff, branch, merge request.

DPD is a four-variant archive set containing twelve delimited files with a documented schema known to be wrong in at least one place, plus a catalogue generator whose slug rules are the real work. Building the pipeline against it first means debugging pipeline and parser simultaneously.

This does not violate §5. That section says the DPD catalogue is built before any provincial **encoding**, which governs encoding order, not acquisition order. §5 records the clarification explicitly so it does not get re-litigated.

Adding DPD second is the first real test of the table-driven claim in §2. See milestone 7.

---

## 2. Repository layout

```
cail-acquire/
  CLAUDE.md                  agent rules, this repo only
  CAIL-ACQUIRE-SPEC.md       this file
  go.mod
  Makefile
  cmd/
    cail-acquire/main.go
  config/
    sources.poll.yaml        poll table data (embedded via config/embed.go)
  internal/
    config/                  poll table load, validation
    manifest/                sources.yaml read/write against the cail-rules working copy
    fetch/                   scrape_anchor, archive
    normalize/               pdf, zip_members
    gate/                    PHI hard stop
    store/                   R2 put, content-addressed
    diff/                    semantic diff report generation
    vcs/                     clone, reset, branch, commit, push, MR create
    status/                  status file build and publish
    catalogue/               DPD extract to three catalogue artifacts
  testdata/
  deploy/
    cail-acquire.service
    cail-acquire.timer
    cail-acquire-alert@.service
```

Subcommands: `poll`, `catalogue`, `status`, `verify-din`.

`sources.yaml` lives in `cail-rules` per §1. The binary maintains a working copy on the droplet and holds no manifest state of its own.

### 2.1 Write boundary

Per §1, the only paths this binary may write in `cail-rules` are `sources.yaml`, `reports/`, and `catalogue/`. Never `knowledge_base/`, never `release.yaml`, never a rule file.

Enforce this in code, not by convention. `manifest` and `vcs` take an allowlist and reject any path outside it. A code path that *could* write outside the allowlist is a bug whether or not it currently triggers.

---

## 3. Verified source facts

Toolchain constants are recorded in §9 and are not restated here except where a build decision depends on one.

### 3.1 NLPDP

| Fact | Value | Confidence |
|---|---|---|
| Host | `gov.nl.ca`, not `health.gov.nl.ca` | VERIFIED |
| Index page (manifest URL) | `https://www.gov.nl.ca/hcs/prescription/covered-specialauthdrugs/` | VERIFIED |
| Payload observed 2026-07-27 | `https://www.gov.nl.ca/hcs/files/Criteria-July-2026.pdf` | VERIFIED as link |
| Anchor text | "Download Information on Special Authorization Drug Products (Last updated on July 16, 2026)" | VERIFIED |
| Structure | One consolidated PDF, all special authorization drugs. Not per-drug | VERIFIED |
| URL stability | None. Filename embeds month and year, changes each revision | VERIFIED |
| Bulletins | Not on `gov.nl.ca`. Provider Notifications redirects to `https://nlpdp.bell.ca/`, authenticated Angular SPA, no public data endpoint | VERIFIED |
| Forms | `https://www.gov.nl.ca/hcs/forms` | VERIFIED |
| Licence | All rights reserved, Government of NL. NL Open Government Licence covers `opendata.gov.nl.ca` only. See §8 | VERIFIED |
| Cadence | Monthly | INFERENCE from one observation |
| `Last-Modified` / `ETag` | Not captured | UNVERIFIED, resolve on first fetch |
| Payload bytes | Link confirmed, bytes not retrieved | UNVERIFIED until milestone 3 |

URL instability is the design driver. A fetcher pointed at `Criteria-July-2026.pdf` breaks silently in August. Per §4, the manifest URL is the index page and the payload is discovered per run.

### 3.2 Health Canada DPD

| Fact | Value | Confidence |
|---|---|---|
| Extract page (manifest URL) | `.../drug-product-database/what-data-extract-drug-product-database.html` on canada.ca | VERIFIED |
| Payload pattern | `https://www.canada.ca/content/dam/hc-sc/documents/services/drug-product-database/<name>.zip` | VERIFIED |
| Status variants | marketed `<name>.zip`, approved `_ap`, cancelled `_ia`, dormant `_dr` | VERIFIED |
| Biosimilar file | href basename is `biosimilar.zip`; link text reads "bios.zip" | VERIFIED |
| Redundancy | `allfiles.zip` already contains `bios.txt`, so no separate biosimilar poll row | VERIFIED from member list |
| Members of `allfiles.zip` | `bios.txt`, `comp.txt`, `drug.txt`, `form.txt`, `ingred.txt`, `package.txt`, `pharm.txt`, `route.txt`, `schedule.txt`, `status.txt`, `ther.txt`, `vet.txt` | VERIFIED |
| Encoding | UTF-8 | VERIFIED |
| Format | Comma-delimited, double-quote qualified | VERIFIED |
| Line endings | Undocumented | UNVERIFIED, detect empirically |
| Join key | `DRUG_CODE`, every file | VERIFIED |
| Product role | `comp.txt` `COMPANY_TYPE`, literal `DIN OWNER`, space separator. No `DIN_OWNER` column exists | VERIFIED |
| Biosimilar indicator | `bios.txt`, `SI_CODE` and `SI_DESC_E`, joined on `DRUG_CODE` | VERIFIED |
| `AI_GROUP_NO` | Not usable as a molecule key | VERIFIED |
| `ther.txt` | Seven columns documented, four observed | VERIFIED |
| Last updated | 2026-07-02 | VERIFIED |
| Refresh schedule | None published | VERIFIED as absent |
| Change feed | Email `osip.sys-bppi@hc-sc.gc.ca`. No RSS or Atom | VERIFIED |
| REST API | `https://health-products.canada.ca/api/drug/`, no auth | VERIFIED |
| API `din=` filter | Ignored. Returns full catalogue sorted by DIN | VERIFIED |
| API rate limit | None published, which is not a guarantee of none | VERIFIED as absent |

Documented column layouts are recorded in §5.1 through §5.3 and are not duplicated here. Treat every layout as provisional per §5.3.

---

## 4. Configuration

### 4.1 Poll table, `config/sources.poll.yaml`

```yaml
sources:
  - id: nlpdp-sa-criteria
    jurisdiction: NL
    publisher: NL Health and Community Services
    fetch: scrape_anchor
    normalize: pdf
    poll_interval: 24h
    staleness_alarm_days: 60
    enabled: true
    scrape:
      link_scope: 'a[href$=".pdf"]'
      anchor_pattern: 'Criteria-[A-Za-z]+-\d{4}\.pdf$'
    strip_rules: []

  - id: hc-dpd-allfiles
    jurisdiction: CA
    publisher: Health Canada
    fetch: scrape_anchor
    normalize: zip_members
    poll_interval: 168h
    staleness_alarm_days: 60
    enabled: false
    scrape:
      link_scope: 'a[href$=".zip"]'
      anchor_pattern: 'allfiles\.zip$'
    strip_rules: []

  - id: ab-idbl
    state: unresolved
  - id: qc-ramq
    state: unresolved
```

DPD is `enabled: false` until milestone 7. The row exists from day one so the source is visible rather than absent.

Alberta and RAMQ appear as `state: unresolved` rows per §4, so CI counts them. They are not poll targets and carry no URL.

Validation at load, all fatal: unknown `fetch` or `normalize` enum value; a poll row with no matching `sources.yaml` entry; a `sources.yaml` entry in `state: active` with no poll row.

`strip_rules` starts empty and stays empty until the soak produces observations. Per §7, every entry carries a comment naming the observed false positive and its date.

### 4.2 Manifest entries, `sources.yaml` in `cail-rules`

```yaml
- id: nlpdp-sa-criteria
  jurisdiction: NL
  publisher: NL Health and Community Services
  url: https://www.gov.nl.ca/hcs/prescription/covered-specialauthdrugs/
  url_kind: index
  url_observed_on: https://www.gov.nl.ca/hcs/prescription/
  url_observed_date: 2026-07-27
  payload_confirmed: false
  last_payload_url: https://www.gov.nl.ca/hcs/files/Criteria-July-2026.pdf
  licence: all-rights-reserved
  licence_ref: https://www.gov.nl.ca/disclaimer/
  raw_hash: UNVERIFIED
  normalized_hash: UNVERIFIED
  state: active

- id: hc-dpd-allfiles
  jurisdiction: CA
  publisher: Health Canada
  url: https://www.canada.ca/en/health-canada/services/drugs-health-products/drug-products/drug-product-database/what-data-extract-drug-product-database.html
  url_kind: index
  url_observed_on: https://www.canada.ca/en/health-canada/services/drugs-health-products/drug-products/drug-product-database.html
  url_observed_date: 2026-07-27
  payload_confirmed: false
  last_payload_url: https://www.canada.ca/content/dam/hc-sc/documents/services/drug-product-database/allfiles.zip
  licence: ogl-canada
  licence_ref: https://healthycanadians.gc.ca/important-eng.php#a6
  raw_hash: UNVERIFIED
  normalized_hash: UNVERIFIED
  state: active
```

`url_kind`, `last_payload_url`, `licence`, and `licence_ref` are the four fields added by §4. `last_payload_url` is provenance only; the fetcher never reads it as input.

`payload_confirmed` flips true on first successful fetch, at milestone 3.

### 4.3 Secrets

systemd `EnvironmentFile`, mode 0600, owner root, group `cail`.

```
R2_ACCOUNT_ID
R2_ACCESS_KEY_ID
R2_SECRET_ACCESS_KEY
R2_BUCKET
GITLAB_TOKEN
GITLAB_PROJECT_ID
```

Environment only. Never a flag, config file, log line, or test fixture. Assert all present at startup and fail before any network call.

---

## 5. Pipeline

Per source, per run:

1. Load poll row. Skip if inside `poll_interval` and not `--force`.
2. Fetch index page. Extract anchor matching `anchor_pattern` within `link_scope`. Resolve to absolute URL.
3. Compare resolved payload URL to `last_payload_url`. A difference is a finding reported in the diff header, never a silent manifest rewrite (§4).
4. Fetch payload. Record HTTP status, final URL after redirects, byte count, `Last-Modified` and `ETag` if present.
5. Normalize to text.
6. Apply `strip_rules`.
7. Hash normalized text. Compare to `normalized_hash`.
8. No change: update status, next source.
9. **PHI gate on normalized text.** Hard stop for that source. Nothing written anywhere.
10. R2 put raw, R2 put normalized.
11. Semantic diff against previous normalized text, fetched from R2 by the prior hash.
12. Branch, commit manifest update plus diff report, push, open MR.
13. Publish status once, after all sources.

Ordering in 10 through 12 is fixed by §3 and is the crash-recovery mechanism. R2 first because content-addressed orphans are harmless. Git last because the manifest hash commit declares the change handled. Do not reorder.

NL produces two independent change signals: payload URL change (monthly, expected) and content hash change. Either opens an MR. A URL change with no hash change means republication without content edit, which is worth knowing.

### 5.1 Exit codes

Aggregate, highest severity wins. Precedence 40, 30, 20, 10, 0.

| Code | Meaning |
|---|---|
| 0 | All sources checked, no change |
| 10 | Change found, MR opened. Normal operation |
| 20 | Fetch, normalize, or store failure |
| 30 | PHI gate fired |
| 40 | Config or manifest error, nothing ran |

`SuccessExitStatus=10` in the service unit per §2. Without it every real detection pages a human.

---

## 6. Components

### 6.1 fetch

Two strategies, enum not per-source code, per §2.

**`scrape_anchor`** Two-hop. GET index page, parse HTML, select by `link_scope`, filter by `anchor_pattern`, resolve relative to base URL, require exactly one match. Zero matches is a failure. Two matches is a failure, not pick-the-first (§2). Then GET the resolved URL.

Capture surrounding anchor text. NL embeds "Last updated on July 16, 2026", recorded in the diff header, never driving the hash (§4).

**`archive`** Milestone 7. Validate before extraction. Reject on size ceiling, member count ceiling, or any member path containing a traversal component. Malformed archive is a fetch failure, not a panic.

`probe_forward` is deferred per §2. It exists for the Ontario ODB page-lag pattern and Ontario is out of scope. Build it when Ontario is added.

Common: 30s connect, 10m total, 3 retries with exponential backoff on 5xx and network error, no retry on 4xx, redirect cap 5. Identifying User-Agent naming CAIL with a contact address. Do not spoof a browser.

### 6.2 normalize

**`pdf`** `pdftotext -layout`, shelled out per §2. `-layout` is not optional: reading-order reflow destroys the row-to-criterion association that a human reviewer reads in the diff. Non-zero exit is a normalization failure. Assert `pdftotext` on PATH at startup.

**`zip_members`** Per-member text, concatenated in sorted member order, each prefixed with a member header line. UTF-8, comma-delimited, double-quote qualified. Detect line endings on first read rather than assuming.

### 6.3 gate

Per §2: fires in the binary as a pre-commit hard stop, before R2 and before git. CI is an enforced merge blocker, not the primary gate. Tuned to over-fire.

Any hit halts that source, writes nothing, returns severity 30.

**SIN.** `\b\d{3}[-\s]?\d{3}[-\s]?\d{3}\b`, then Luhn mod-10. Luhn removes roughly nine in ten false positives on arbitrary nine-digit runs.

**NL MCP.** 12 digits, `\b\d{12}\b`, plus a spaced-grouping variant. High false-positive potential against a number-dense document. Accept it. A DIN is 8 digits, so no collision with the most common numeric token in these files.

**Supporting.** Phone, postal code adjacent to name-shaped tokens, date of birth adjacent to name-shaped tokens.

Expected behaviour on a public criteria document is zero hits, permanently. A hit is either a publisher accident or a normalizer bug, and both need a human immediately.

Per §2, any jurisdiction whose health card format is not implemented is flagged `gate_partial: true` in the status file. Today that is every province except NL.

Never commit a real positive to `testdata/`. Synthetic fixtures only.

### 6.4 store

`aws-sdk-go-v2` against R2, configured per §3 and §9.

```
raw/<source_id>/<raw_hash>
norm/<source_id>/<normalized_hash>
status/current.json
```

Bucket is private per §3. No public access, no presigned URLs, no custom domain. It holds a copy of a document that is copyright Government of Newfoundland and Labrador (§8).

Store on normalized-hash change only, with the three §3 qualifications: first fetch always stores, store on any hash change rather than judged-meaningful change, normalize before hashing.

Two objects per change. Recording a hash without storing its payload breaks provenance permanently (§3).

### 6.5 diff

The artifact a human encoder reads to decide what to re-encode.

**`pdf`:** section-aware. Headings appeared, disappeared, or changed body. Criteria-bearing paragraphs changed. Table changes. Markdown output.

**`zip_members`:** member-hash level only. Which members changed. No body diff. INFERENCE: full-body diffing a twelve-file extract produces noise, and the real downstream action is a catalogue rebuild.

Header carries: source id, previous hash, new hash, previous payload URL, new payload URL, anchor text, fetch timestamp, `Last-Modified` if present, byte delta.

### 6.6 vcs

INFERENCE: shell out to `git` rather than use `go-git`. The operations are clone, fetch, reset, checkout, commit, push. The droplet has git. `go-git` adds a dependency and a class of divergence bugs for no gain at two sources.

Working copy at `/var/lib/cail-acquire/cail-rules`, cloned once at bootstrap. Each run: `git fetch origin`, `git reset --hard origin/main`, branch from there. Never merge, never push to main.

Branch `src/<source_id>/<normalized_hash short 12>`. Existing branch on origin means the MR is already open, so skip and exit clean for that source (§3).

MR via `POST /api/v4/projects/:id/merge_requests` with a `PRIVATE-TOKEN` header. One endpoint, no client library.

Credential per §2: Project Access Token, `api` scope, Developer role, `main` protected with merge restricted to Maintainers. Verified at bootstrap by attempting a push to `main` and confirming rejection.

MR contents, three things per §1: manifest hash update, `reports/<source_id>/<date>-<hash short 12>.md`, and in the description the list of encoded rule IDs in `cail-rules` citing that `source_id`. The third is what makes it a work order. Title `[acquire] <source_id> changed <YYYY-MM-DD>`.

### 6.7 catalogue

Milestone 11, DPD only. Three artifacts per §5: `catalogue/ingredients/<slug>.json`, `catalogue/din-index.json`, `catalogue/_meta.json`. Generated, never hand-edited.

Parser constraints, all from §5.1 through §5.3:

- Join on `DRUG_CODE`.
- Product role from `comp.txt` `COMPANY_TYPE`, matching literal `DIN OWNER`. Space, not underscore.
- Biosimilar status from `bios.txt` `SI_CODE` / `SI_DESC_E`.
- `AI_GROUP_NO` is not a molecule key.
- Assert column count per file at ingest, fail loudly on divergence. `ther.txt` is the known case at four observed versus seven documented. The assertion is how the others get found.

Slug generation is the real work. Deterministic, stable across extracts, unit-tested against salts, hydrates, combination products, and multi-word ingredient names.

Ingest scope is unresolved. See open item 5 in §10 and §9.3 below.

### 6.8 verify-din

Milestone 12. Single-DIN fallback against the DPD JSON API. Never bulk (§5.4).

The API ignores `din=`. Resolve DIN to `drug_code` first, or filter client-side. Cache results. Throttle conservatively; no published rate limit is not a guarantee of none.

A DIN present in the API and absent from the catalogue is marked `EXTRACT_LAGGED`. Do not fail the build, do not invent a record (§5.4).

### 6.9 status

Built from run result plus manifest, published to R2, never committed (§1).

```json
{
  "generated_at": "2026-07-27T09:00:00Z",
  "sources": [
    {
      "id": "nlpdp-sa-criteria",
      "last_checked": "2026-07-27T09:00:00Z",
      "last_change": null,
      "last_payload_url": "https://www.gov.nl.ca/hcs/files/Criteria-July-2026.pdf",
      "consecutive_failures": 0,
      "normalized_hash": null,
      "staleness": "ok",
      "gate_partial": false,
      "open_mr": null
    }
  ]
}
```

Runs unconditionally, even after failures. A run that fails to publish status is worse than one that fails to fetch, because the failure becomes invisible.

---

## 7. Failure handling

- A failing source does not stop the run.
- Failures increment a status counter, do not open MRs, do not touch the manifest.
- Staleness fires independently of failure state. A source can be healthy and stale. For DPD that is normal given no published refresh schedule and an observed months-scale gap.
- systemd will not start a service unit already running, so no lock file is needed on the timer path.

---

## 8. Testing

- **Unit, no network.** Normalizers against golden fixtures. `scrape_anchor` against an `httptest` server serving a saved copy of the NL index page, including zero-match and multi-match failure cases. Gate against synthetic positives and negatives, including Luhn-valid and Luhn-invalid nine-digit runs. Slug generation against the hard-case set.
- **Integration, build tag `integration`.** Live fetch of each source, assert 200 and non-trivial body. Manual and weekly CI, never in the normal test run.
- **Golden fixtures** are read by a human before commit. `make regen-fixtures` exists; its output is reviewed, never accepted blindly.
- **Store and vcs** are interfaces, faked in unit tests, exercised for real only in the soak.
- **Write boundary** has its own test: attempt a write to `knowledge_base/` through the `manifest` and `vcs` interfaces and assert rejection.

---

## 9. Environment and deployment

### 9.1 Development

- Go 1.26.5 (§9).
- `poppler-utils` locally, matching the deploy image major version.
- Dependencies: `aws-sdk-go-v2` (s3, config, credentials), a YAML library, an HTML parser, standard library. Keep the list readable aloud in a pharma security review.
- Lint: `go vet` plus `staticcheck`, enforced in CI.
- `--dry-run --source=nlpdp-sa-criteria` must work with no R2 or GitLab credentials present. If it does not, the layering is wrong.

### 9.2 Target

Single DigitalOcean droplet, Ubuntu 24.04 LTS, 1 GB tier per §9.

INFERENCE: run the binary directly under systemd, not in Docker. Docker buys isolation you do not need on a single-purpose droplet and adds a layer to every debugging session.

Provisioning:

1. User `cail`, no shell login.
2. `apt install poppler-utils git ca-certificates`.
3. `/var/lib/cail-acquire/` owned by `cail`, mode 0750.
4. `/etc/cail-acquire/env` mode 0600, owner root, group `cail`.
5. `/usr/local/bin/cail-acquire`.
6. Clone `cail-rules` as the bot user. Confirm push to `main` is rejected (§2).
7. `systemctl enable --now cail-acquire.timer`.

Build on a workstation with `CGO_ENABLED=0 GOOS=linux`. No toolchain on the droplet. Deploy is `scp` plus `systemctl restart`.

### 9.3 systemd

```ini
[Unit]
Description=CAIL source acquisition
After=network-online.target
Wants=network-online.target
OnFailure=cail-acquire-alert@%n.service

[Service]
Type=oneshot
User=cail
EnvironmentFile=/etc/cail-acquire/env
WorkingDirectory=/var/lib/cail-acquire
ExecStart=/usr/local/bin/cail-acquire poll
SuccessExitStatus=10
TimeoutStartSec=30m
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/cail-acquire
```

```ini
[Timer]
OnCalendar=*-*-* 09:00
Persistent=true
RandomizedDelaySec=15m

[Install]
WantedBy=timers.target
```

`Persistent=true` per §2. The daily timer is the outer loop; per-source `poll_interval` is enforced inside the binary, so the weekly DPD source is a cheap no-op six days out of seven.

### 9.4 Alerting

`cail-acquire-alert@.service`, oneshot, sends unit name and last 50 journal lines. INFERENCE: transactional email API is sufficient. Do not build a monitoring stack for one binary.

Alert on any non-zero exit other than 10, any staleness alarm, and absence of a status file update for more than 48 hours. The third catches the timer not firing at all, which the other two cannot.

Transport is unchosen. Open item 10 in §10. Decide before milestone 10 or the soak runs unwatched.

### 9.5 Backup

The droplet holds no unique state. `cail-rules` is on GitLab, payloads are in R2, the binary is rebuildable. Do not back up the droplet. Verify the §9.2 checklist by rebuilding once from scratch before the soak ends.

---

## 10. Milestones

Mirrors the build sequence in §7, steps 1 through 12. Step 13 of that sequence is encoding tocilizumab/Avtozma, which happens in `cail-rules` and is out of scope for this repo.

Do not start a milestone before the prior acceptance criterion is met and committed.

| # | Milestone | Done when |
|---|---|---|
| 1 | ✅ `DECISIONS.md` corrections committed | Complete 2026-07-27 |
| 2 | `scrape_anchor` resolves NL criteria URL | Resolves the current `Criteria-<Month>-<Year>.pdf`. Zero-match and multi-match both fail loudly |
| 3 | Fetch NL payload, `pdftotext -layout`, hash | Stable hash across two consecutive runs. No R2, no git. `payload_confirmed` flips true |
| 4 | R2 store | Raw and normalized objects at content-addressed keys. Rerun writes identical keys. Bucket confirmed private |
| 5 | **Seven-day soak, NL only** | Seven days of hashes. Churn rate measured. `strip_rules` tuned against observation and committed |
| 6 | PHI gate | Fires on synthetic SIN and MCP positives. Zero hits across the soak corpus |
| 7 | Add DPD | **Architectural test.** Should require a config row, one fetch strategy, one normalizer, and nothing else. If anything else must change, the design failed and that is worth knowing on source two rather than source six |
| 8 | Semantic diff | Report generated for a real observed change, or a synthetic edit of a captured payload |
| 9 | Branch, push, MR | MR opens with the three required contents. Push to `main` confirmed rejected |
| 10 | Status and alerting | Status publishes. A forced failure produces an alert |
| 11 | `catalogue` subcommand | Three artifacts. Slug generation passes hard-case fixtures. Column-count assertion catches `ther.txt` |
| 12 | `verify-din` | Resolves a known DIN correctly, working around the ignored `din=` filter |

Milestones 2 through 4 are the immediate work.

**Milestone 5 is a hard gate** per §7. Everything after it depends on knowing the churn rate. Building the diff and MR path first means tuning normalization inside a pipeline that already opens merge requests, which trains a human to ignore merge requests.

§7 carries an OPEN item on whether this gate should be enforced mechanically rather than by instruction, on the grounds that an instruction not to build past step 5 will be violated when a session runs long. INFERENCE: a `.milestone` file read by the Makefile, refusing to compile `internal/diff` and `internal/vcs` until it advances, would make the gate real. Not yet decided.

---

## 11. Open items

Consolidated open items live in §10 and are authoritative. Those bearing on this repo:

| §10 item | Bears on |
|---|---|
| 2. `gov.nl.ca` `Last-Modified` / `ETag` unresolved | Milestone 3. Capture on first fetch. A cheap pre-hash signal if present, never authoritative |
| 3. NL monthly cadence inferred from one observation | Milestone 5. Confirm across two revisions during the soak |
| 4. DPD line endings undocumented | Milestone 7. Detect empirically |
| 5. Extract status scope undecided | Milestone 11. `allfiles.zip` is marketed-only; approved-not-yet-marketed products are in `allfiles_ap.zip`, and newly launched biosimilars are exactly that case. §5 says ingest the whole extract with no filtering, which argues for both variants tagged by source |
| 7. Retention scope | Belongs in `store` as an R2 lifecycle rule. Undecided means unlimited retention by default, a decision made by omission |
| 10. Alert transport unchosen | Milestone 10 |
| 11. Soak gate enforcement mechanism | Milestone 5. See §10 above |

Items 1, 6, 8, and 9 in §10 do not bear on this repo.

---

## 12. Deliberate exclusions

**No workflow engine.** Two sources, idempotent by content hash, crash recovery by branch name. Durable execution buys nothing here and costs a server, a database, and a component in every future security review.

**No `probe_forward`.** Deferred per §2. Ontario is out of scope.

**No bulletin acquisition.** NLPDP bulletins are behind an authenticated SPA (§4). Do not build a scraper against it.

**No API, engine, or serving layer.** Those live in `cail-rules` and are later concerns.

**No LLM calls in this binary, ever** (§2). LLM-assisted extraction is a `cail-rules` review-queue tool after the schema is stable. An LLM in the acquisition path automates exactly the human step this service is named to protect.