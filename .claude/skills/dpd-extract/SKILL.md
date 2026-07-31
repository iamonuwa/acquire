---
name: dpd-extract
description: How to parse the Health Canada Drug Product Database bulk extract and generate the DIN-to-ingredient catalogue. Use this skill whenever the work touches the DPD extract, `internal/catalogue`, the `catalogue` subcommand, `din-index.json`, ingredient slug generation, `verify-din`, or any of the extract's delimited files (`drug.txt`, `comp.txt`, `ingred.txt`, `bios.txt`, `ther.txt` and the rest), and also whenever someone needs to resolve a DIN to an ingredient or asks which company owns a product. Health Canada's published documentation for this extract is known to be wrong in at least one place, and several fields do not mean what their names suggest, so consult this before writing parser code.
---

# Parsing the Health Canada DPD extract

The DPD catalogue is the join key for the whole system. Provinces cite DIN and brand name, encoded rules key by ingredient, and this catalogue bridges them. A wrong entry here propagates into every rule that resolves through it.

The governing constraint: **read actual column layouts from the extract, never from Health Canada's documentation.** The documentation is confirmed wrong for `ther.txt` and is assumed wrong elsewhere. Assertions are how the other cases get found.

Column layouts as documented are in `references/column-layouts.md`. Read that file when writing a parser for a specific table. Treat every layout there as a claim to be verified against the file, not a fact.

## Acquire the extract

The extract page is on canada.ca and payloads live under a `content/dam` path. The manifest URL is the index page, not a payload, because payload discovery goes through `scrape_anchor`.

Four product-status variants exist per table:

| Variant | Suffix | Contains |
|---|---|---|
| Marketed | `<name>.zip` | Products currently marketed |
| Approved | `<name>_ap.zip` | Approved, not yet marketed |
| Cancelled | `<name>_ia.zip` | Cancelled or inactivated |
| Dormant | `<name>_dr.zip` | Dormant |

`allfiles.zip` bundles the twelve marketed tables. It already contains `bios.txt`, so a separate biosimilar poll row is redundant.

**Trap:** the extract page's link text reads `bios.zip`, but the href basename is `biosimilar.zip`. Requesting `bios.zip` returns 404. Read hrefs, not link text.

**Unresolved scope question.** `allfiles.zip` is marketed-only. A DIN cited in a payer bulletin for an approved-but-not-yet-marketed product will not resolve against a marketed-only catalogue, and newly launched biosimilars are exactly that case. The decision record says ingest the whole extract without filtering, which argues for ingesting both variants and tagging records by source. This is an open item. If the work requires a decision, surface it rather than silently picking one.

## Parse mechanics

- Encoding is **UTF-8**. Not ISO-8859-1, despite what you might assume for a Canadian government text export.
- Comma-delimited, double-quote qualified. Use a real CSV reader, not `strings.Split`.
- Line endings are undocumented. Detect them on first read rather than assuming.
- `DRUG_CODE` is the join key across every file. Not DIN.

## Assert column counts at ingest

Before parsing any table, assert its actual column count against the expected count and fail loudly on divergence.

This is not defensive habit, it is the mechanism by which documentation errors get discovered. `ther.txt` is documented with seven columns and observed with four. That one was found by hand. The assertion is how the next one gets found without anyone looking.

Fail the build. Do not skip the row, do not pad missing columns, do not infer which four of the seven documented fields are present.

## Fields that do not mean what they appear to

Three traps, each of which produced a wrong entry in the decision record before being corrected.

**Product role.** There is no `DIN_OWNER` column anywhere in the extract. Product role lives in `comp.txt` as `COMPANY_TYPE`, carrying the literal space-separated string `DIN OWNER`. Code that splits on an underscore, or that looks for a `DIN_OWNER` column, is wrong.

**Biosimilar status.** Not a flag on `drug.txt`. It is a separate file, `bios.txt`, with fields `SI_CODE` and `SI_DESC_E`, joined on `DRUG_CODE`. The broader principle is the durable one: do not trust a biosimilar indicator alone, derive product role from the company relationship.

**`AI_GROUP_NO`.** Present on `drug.txt` and looks like a molecule grouping key. It is not usable as one. Do not group ingredients by it.

## Generate the catalogue

Three artifacts, all generated, never hand-edited:

| Artifact | Contents |
|---|---|
| `catalogue/ingredients/<slug>.json` | One per ingredient: DINs, brands, companies, strengths, forms, routes, schedule, status, product role |
| `catalogue/din-index.json` | Flat DIN to ingredient slug. The file provincial encoding reads |
| `catalogue/_meta.json` | Extract date, source hash, generator version |

Slug generation is the real work in this subcommand. Slugs must be deterministic and stable across extracts, because a slug that changes between extracts silently breaks every rule that references it.

Unit-test slug generation against the hard cases before trusting it:

- Salts and esters, where the same molecule appears under several ingredient strings
- Hydrates
- Combination products with several active ingredients
- Multi-word ingredient names
- Names differing only by case, punctuation, or whitespace

## Verifying a single DIN

The DPD JSON API is a single-DIN verification fallback only, never a bulk source. No authentication is required.

The API **honors** the `din=` filter — `?din=<DIN>` returns just the matching product. It was previously observed ignoring the filter and returning the entire catalogue, so **filter client-side for an exact match anyway**; that stays correct whichever way the API behaves. Cache results.

No rate limit is published. That is not a guarantee that none exists. Throttle conservatively.

If a bulletin cites a DIN that is absent from the catalogue but present in the API, mark the entry `EXTRACT_LAGGED`. Do not fail the build and do not invent a record. The extract lags the online database; that gap is expected and should be represented, not papered over.

## Cadence

No refresh schedule is published. The extract has been observed advancing on a months-scale, irregular basis. Poll weekly with a 60-day staleness alarm, and treat the Health Canada extract mailing list as the change feed. There is no RSS or Atom feed.

A DPD source can be healthy and stale at the same time. That is the normal condition, not an error.

## When a DIN looks wrong

Verify it against the catalogue or the API before recording it anywhere. Do not carry a DIN forward from a bulletin, a note, or a prior conversation on the assumption that it was checked.

A DIN carried through project notes without verification once reached a locked decision file attached to entirely the wrong molecule. It survived several rounds of review because it looked plausible and nobody re-checked it. That is the failure mode this rule exists to prevent.