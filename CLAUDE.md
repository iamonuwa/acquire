# CAIL ACQUIRE

This file governs **only** the acquisition binary. If a task is about encoding a drug, designing a rule schema, or building the engine, it does not belong in this repo and you should say so rather than doing it here.

---

## What this binary does

Polls a fixed table of Canadian payer source URLs. Detects content change by normalized hash. Stores payloads in Cloudflare R2. Opens a merge request in `cail-rules` as a **work order for a human encoder**.

It acquires and reports. It does not encode, interpret, or decide anything about drug coverage.

The service is deliberately named "acquisition service," not "ETL," to prevent anyone automating the human encoding step. That includes you.

**Sources in scope:** two. `nlpdp-sa-criteria` and `hc-dpd-allfiles`. Nothing else.

**Subcommands:** `poll`, `catalogue`, `status`, `verify-din`.

---

## The write boundary

This binary runs against a working copy of `cail-rules`. That makes the boundary the single most important rule in this file.

**Writable:** `sources.yaml` (hash and provenance fields only), `reports/`, `catalogue/`.

**Never:** `knowledge_base/`, `release.yaml`, any rule file, `DECISIONS.md`, or any CLAUDE.md.

If a code path could write outside the three allowed paths, that is a bug regardless of whether it currently triggers.

---

## Hard rules

1. **Never invent a URL, filename, column name, figure, or date.** If you do not have a verified value, write `UNVERIFIED` or `UNRESOLVED`. Those are valid committed values that CI counts. A plausible-looking guess is the worst thing you can produce in this repo.

2. **Never hardcode a payload URL.** Both sources publish at URLs that change. The manifest holds an **index page** URL; the payload URL is discovered per run by `scrape_anchor`. If you find yourself typing `Criteria-July-2026.pdf` into a source file, stop.

3. **Never trust publisher documentation over the actual file.** Health Canada documents seven columns for `ther.txt`; the file has four. Assume the documentation is wrong elsewhere. Parse what is there, assert column counts at ingest, fail loudly on divergence.

4. **Never weaken the PHI gate to reduce false positives.** Over-firing is correct. A false positive costs one alert. A false negative puts patient data in a git object, which is unrecoverable in practice.

5. **The PHI gate fires before any write.** In the binary, before R2 and before git. Not on PR, not in CI. CI is a merge blocker, not the primary gate.

6. **Pipeline order is fixed.** R2 put, then diff, then branch, then commit, then push, then MR. R2 first because content-addressed orphans are harmless. Git last because the manifest hash commit declares the change handled. This ordering is the entire crash-recovery mechanism. Do not reorder for elegance.

7. **Branch name carries the normalized hash.** A branch that already exists on origin means the MR is already open. Skip the source, exit clean. Do not add state tracking to replace this.

8. **No LLM calls in this binary. Ever.**

9. **No binary payloads in git.** Repo is text only. PDFs and ZIPs go to R2.

10. **Fail loudly, never silently.** Zero anchor matches is a failure. Two anchor matches is a failure, not pick-the-first. Malformed archive, column mismatch, missing env var: all failures. Never degrade to a guess.

11. **Secrets from environment only.** Never a flag, config file, log line, or test fixture. Assert all present at startup, fail before any network call.

12. **No new dependency without justification.** Current list: `aws-sdk-go-v2` (s3, config, credentials), a YAML library, an HTML parser, standard library. This has to be readable aloud in a pharma security review.

13. **Golden fixtures are read by a human before commit.** `make regen-fixtures` exists. Its output is reviewed, never accepted blindly.

14. **The bot pushes, it does not merge.** Do not add merge capability, do not suggest it.

15. **Do not build past milestone 5.** The seven-day soak is a hard gate. Building the diff and MR path before churn is measured means tuning normalization inside a pipeline that already opens merge requests.

16. **Do not add sources.** Two. A third is a human product decision, not a refactor.

17. **Every `strip_rule` carries a comment naming the observed false positive and its date.** Rules invented ahead of observation are how a real change gets hashed away.

18. **Never commit a real PHI positive to `testdata/`.** Synthetic fixtures only.

---

## Verified constants

Verified 2026-07-27 against publisher pages. Do not re-derive. Do not "correct" from memory.

### NLPDP (`nlpdp-sa-criteria`)

| | |
|---|---|
| Manifest URL (index) | `https://www.gov.nl.ca/hcs/prescription/covered-specialauthdrugs/` |
| Host | `gov.nl.ca`. **Not** `health.gov.nl.ca` |
| Payload shape | One consolidated PDF, all drugs. Not per-drug |
| Payload URL as of 2026-07-27 | `https://www.gov.nl.ca/hcs/files/Criteria-July-2026.pdf` — date-stamped, changes each revision, **never hardcode** |
| Anchor pattern | `Criteria-[A-Za-z]+-\d{4}\.pdf$` |
| Anchor text | Carries a revision date. Record in the diff header. Never drives the hash |
| Bulletins | Behind `https://nlpdp.bell.ca/`, authenticated SPA. Not harvestable. Out of scope |
| Licence | All rights reserved, Government of NL. R2 bucket stays private |
| Cadence | Monthly, INFERRED from one observation. Confirm during the soak |
| `Last-Modified` / `ETag` | UNVERIFIED. Capture on first fetch |

### Health Canada DPD (`hc-dpd-allfiles`)

| | |
|---|---|
| Manifest URL (index) | `https://www.canada.ca/en/health-canada/services/drugs-health-products/drug-products/drug-product-database/what-data-extract-drug-product-database.html` |
| Payload pattern | `https://www.canada.ca/content/dam/hc-sc/documents/services/drug-product-database/<name>.zip` |
| Refresh schedule | None published. Last updated 2026-07-02 |
| Change feed | Email `osip.sys-bppi@hc-sc.gc.ca`. No RSS or Atom |

Extract mechanics, column layouts, product role, biosimilar joins, and the API's ignored `din=` filter are in the **`dpd-extract` skill**, which loads when the work touches `internal/catalogue` or the extract files. They are not repeated here, because a fact stored in three places gets corrected in one.

### Toolchain

| | |
|---|---|
| Go | 1.26.5 |
| R2 client | region `auto`, static credentials, `o.BaseEndpoint = aws.String("https://<account_id>.r2.cloudflarestorage.com")` |
| PDF | `pdftotext -layout`, shelled out. poppler-utils, GPL, Ubuntu 24.04 LTS main |
| GitLab token | `api` scope, Developer role. `read_repository` is insufficient |
| Scheduler | systemd timer, `Persistent=true`. Not cron |
| PHI patterns | NL MCP 12 digits. SIN 9 digits with Luhn check |

---

## Traps

Places where training data actively misleads. Each was wrong in a draft of this project.

1. **`EndpointResolverWithOptions` is deprecated.** Most circulating R2 and S3 examples use it. Use `BaseEndpoint` with endpoint resolution v2.

2. **`bios.zip` does not exist.** Health Canada's link text reads "bios.zip"; the href basename is `biosimilar.zip`. Requesting `bios.zip` returns 404. `allfiles.zip` already contains `bios.txt`, so you likely need neither.

3. **The DPD API ignores `din=`.** Passing it returns the entire catalogue sorted by DIN. It looks like it should work. Resolve DIN to `drug_code`, or filter client-side.

4. **`pdftotext` without `-layout` is wrong here.** The default reflows to reading order and destroys the row-to-criterion association in tabular criteria documents. The flag is not optional.

5. **`health.gov.nl.ca` appears in older documents.** Current host is `gov.nl.ca`.

6. **`SuccessExitStatus=10` in the systemd unit is deliberate.** Exit 10 means a change was found and an MR opened, which is normal operation. Removing it makes every real detection page a human.

---

## Exit codes

Aggregate across sources, highest severity wins. Precedence 40, 30, 20, 10, 0.

| Code | Meaning |
|---|---|
| 0 | All sources checked, no change |
| 10 | Change found, MR opened. **Normal operation** |
| 20 | Fetch, normalize, or store failure |
| 30 | PHI gate fired |
| 40 | Config or manifest error, nothing ran |

---

## Current position

Milestone 2 of 12. Full table and acceptance criteria in `CAIL-ACQUIRE-SPEC.md` §12.

- **1** ✅ `DECISIONS.md` corrections committed (`cail-rules` @ 98b6b54)
- **2** `scrape_anchor` resolves the NL criteria URL ← next
- **3** Fetch, `pdftotext -layout`, stable hash
- **4** R2 store
- **5** Seven-day soak ← **gate, do not build past this**
- **7** Add DPD. **Architectural test:** should require a config row, one fetch strategy, one normalizer, nothing else. If anything else must change, stop and say so

---

## When unsure

Say `UNVERIFIED` and stop. Correctness means never producing a confident wrong answer. Here a confident wrong answer becomes a committed fact a human later builds a payer determination on top of.

Ask before: adding a dependency, changing pipeline order, adding a source, touching the PHI gate, editing `DECISIONS.md`, or writing anywhere in `cail-rules` outside `sources.yaml`, `reports/`, and `catalogue/`.