---
name: soak-analysis
description: Analyse hash churn from the acquisition soak and write justified strip rules. Invoke this deliberately as /soak-analysis when the soak has run and you need to decide whether the churn rate is low enough to proceed past the milestone 5 gate. Also use it whenever a source is opening merge requests for changes that turn out to be nothing, when someone proposes adding a strip rule, or when normalization appears to be hiding a real change. This is the gate on the whole build sequence and it is judgement rather than a script, so work through it rather than eyeballing the hashes.
---

# Soak analysis

The soak exists to answer one question: when this source's hash changes, how often is that a real criteria change and how often is it noise?

You cannot know the answer in advance, which is why the gate exists. Tuning normalization inside a pipeline that already opens merge requests trains a human to ignore merge requests, and once that happens the whole system is decorative.

This is a deliberate, human-initiated analysis. Do not run it automatically and do not let its conclusions be inferred from a single day.

## What you need before starting

- At least seven consecutive days of recorded hashes for the source.
- The normalized text objects in R2 for every distinct hash observed.
- The raw payloads for the same, for cases where normalization itself is suspect.

If any day is missing, say so and do not extrapolate. A six-day soak with one gap is not a seven-day soak; the missing day is exactly where a weekly publication pattern would hide.

## Step 1: Count the churn

For the source, produce:

- Number of days observed.
- Number of distinct normalized hashes.
- Number of hash transitions.
- For each transition, the date and the two hashes.

State the raw numbers before interpreting them. A source that changed zero times in seven days has told you nothing about its churn rate, and that is a finding: the soak needs to run longer or span a known publication event.

## Step 2: Classify every transition

For each transition, retrieve both normalized texts from R2 and diff them. Then classify.

**Volatile.** The document is substantively identical and the difference is an artifact of rendering or delivery. Common signatures:

- A generation or "printed on" timestamp in a header or footer.
- A session identifier or cache-busting token embedded in the text layer.
- A page counter or total that shifts because of pagination rather than content.
- Reordering of elements that carry no meaning in order.

**Real.** Something a human encoder would need to act on:

- Criteria text, eligibility conditions, or clinical thresholds.
- Drug names, strengths, or product listings.
- Effective dates, renewal periods, or coverage end dates.
- Prescriber restrictions or required documentation.

**Ambiguous.** Anything you cannot confidently place. Leave it ambiguous. Do not resolve an ambiguous case toward volatile because that is the classification that reduces future alerts.

Be careful with dates. A rendering timestamp is volatile. An effective date is the single most consequential field in a criteria document. They look alike in a diff and mean opposite things.

## Step 3: Write strip rules only for what you observed

A strip rule may be written only against a transition you actually classified as volatile in step 2.

Every rule carries a comment naming the observed false positive and the date it was seen:

```yaml
strip_rules:
  # Footer render timestamp, observed 2026-08-03, hash a1b2c3 -> d4e5f6
  - pattern: 'Generated on \d{1,2} [A-Za-z]+ \d{4} at \d{2}:\d{2}'
```

Rules invented ahead of observation are how a real change gets normalized away and never opens a merge request. That failure is silent and permanent: the hash matches, no merge request opens, and nobody learns the criteria changed.

Write the narrowest pattern that covers the observed case. A pattern that strips all four-digit years to eliminate one footer timestamp will also strip effective dates.

## Step 4: Re-run against the whole soak corpus

After adding rules, re-normalize and re-hash every payload captured during the soak.

Two things must hold:

- Every transition you classified as volatile collapses.
- Every transition you classified as real still produces a hash change.

If a real transition stops producing a change, the rule is too broad. Narrow it and repeat. This check is the entire point of adding rules against a captured corpus rather than against live traffic.

## Step 5: Decide the gate

Report the post-rule churn rate and make a recommendation.

Proceeding is reasonable when the remaining transitions are all real or ambiguous, and the expected merge request volume is low enough that a human will actually read each one. For a monthly-revised source that means roughly one merge request per revision.

Do not proceed when volatile transitions remain unexplained, when a real transition was suppressed by a rule, or when the soak observed no transitions at all and therefore measured nothing.

Not proceeding is a legitimate outcome. Extending the soak costs days. Shipping a pipeline that cries wolf costs the credibility of every future alert, and that does not come back.

## Step 6: Record it

The churn rate and the classification decisions belong in the decision record, not only in the poll table comments. A future maintainer needs to know not just what the rules are but what was observed that justified them.

## A note on ambiguous cases

Ambiguous transitions are not failures of the analysis. They are the normal result of reading a diff without domain context.

Take them to a human who knows the source. On a payer criteria document, that means someone who can say whether a changed line altered what a patient must satisfy. Do not resolve them by inspection alone, and do not let them accumulate silently into an implicit "probably fine."