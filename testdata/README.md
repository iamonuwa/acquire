# testdata fixtures

Hand-written HTML pages for the `internal/fetch` scrape_anchor tests. The tests
serve these from a local `httptest` server, so they need no network and contain
no PHI or secrets.

The `happy` fixture uses the real values seen on the NL page (`Criteria-July-2026.pdf`,
"… Last updated on July 16, 2026") so the test pins the actual filename shape.

`make regen-fixtures` fetches the live NL page into `nlpdp_index_happy_live.html`
for a human to review before use. It never touches the synthetic fixtures below.

| File | Purpose |
|---|---|
| `nlpdp_index_happy.html` | exactly one matching Criteria PDF (+ decoy links) |
| `nlpdp_index_zero.html` | .pdf links present, none match the pattern |
| `nlpdp_index_multi.html` | two matching PDFs → must fail loudly |
| `nlpdp_index_relative.html` | a dot-relative href, for URL resolution |
