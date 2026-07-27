# testdata fixtures

Fixtures for `internal/fetch` scrape_anchor unit tests. Unit tests run with **no network** (SPEC §8): an `httptest` server serves these bytes.

## Provenance

All four HTML files here are **representative synthetic fixtures**, not live captures. They are deliberately small and hand-authored to exercise the anchor resolution logic (link_scope selection → anchor_pattern filtering → relative-URL resolution → exactly-one contract). None contains PHI or secrets (CLAUDE rules 9, 18); the payload PDF is never fetched.

The **happy** fixture uses the values verified 2026-07-27 (SPEC §3.1 / `cail-rules` DECISIONS.md): payload filename `Criteria-July-2026.pdf` and the anchor text "… (Last updated on July 16, 2026)". These are committed verified constants, used here so the unit assertion pins the real filename shape.

## The authoritative live capture

`make regen-fixtures` (integration-tagged, `TestCaptureNLIndex`) fetches the **live** NL index page and writes it to `nlpdp_index_happy_live.html` for a human to review before commit (CLAUDE rule 13). It never overwrites the synthetic fixtures below, which must stay stable for deterministic unit tests.

| File | Purpose |
|---|---|
| `nlpdp_index_happy.html` | exactly one matching Criteria PDF (+ decoy PDFs/links) |
| `nlpdp_index_zero.html` | in-scope PDFs present, none match the anchor pattern |
| `nlpdp_index_multi.html` | two matching Criteria PDFs → ambiguous, must fail loudly |
| `nlpdp_index_relative.html` | matching anchor with a dot-relative href, for base resolution |
