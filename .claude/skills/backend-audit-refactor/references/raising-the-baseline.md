# Raising the Baseline — make quality a property of the system

This is the most socially dangerous file in the playbook. It exists because
"the engineers here are unprofessional" is a *conclusion*, and it's the one place
where you can be confidently wrong in a way that costs you your credibility and
the fix. The discipline that governs the whole playbook — never theorize before
you measure — applies to people most of all, precisely because that's when it's
hardest to hold.

Two separate problems, handled in order: **verify the diagnosis**, then **build
the antidote**.

---

## Part 1 — Verify before you indict

Bad *output* has several possible causes. Only one of them is bad *engineers*.
Rule out the others first, because they're more common and the fix is different:

- **Missing scaffolding (most common).** No tests, no CI, no review bar, no SLOs.
  Competent people ship sloppy work in an environment that never told them the
  standard or never caught the miss. This is an environment problem wearing a
  people-problem costume. It is *not* a character flaw and it's the likeliest
  answer.
- **Constraint under fire.** A real-time/trading platform ships under deadline
  and incident pressure. That N+1 might be someone's 2am hotfix during an outage,
  not their idea of good code. `git blame` the commit and read the message before
  you read the person.
- **Chesterton's fence, applied to people.** The weird code might be the one who
  *did* care, working around something you can't see yet. Understand why it's
  there before concluding the author was careless.
- **Then, genuinely: below-standard work from someone who should know better.**
  This exists. It's just last on the list, not first.

**The tell that separates the last case from the rest:** a *pattern that persists
after clear, fair feedback* — not a single ugly file. One bad commit is noise.
The same defect class, repeated, after it's been flagged, with no engagement —
that's signal. Until you have that, "unprofessional" is a hypothesis you haven't
run `EXPLAIN ANALYZE` on. Treat it exactly as skeptically as you'd treat "the
database is just slow."

**The credibility constraint.** Coming in as the frontend person newly auditing
the backend and concluding the backend engineers are the problem is the
*maximally* suspicious version of that claim — to them and to your manager.
Contempt is legible: in review tone, in Slack, in the questions you ask in
standup. It torches your credibility faster than any slow query loses a fill. The
audit only lands if it reads as *"here's what the system is missing,"* never
*"here's who's bad."* This is not only diplomacy — the system framing is also the
one that actually gets the code fixed, because most people meet a clear bar once
one exists.

---

## Part 2 — The antidote is a system, not better people

You don't fix bad work by wishing for good engineers. You build an environment
where bad work *can't ship silently*. Quality stops being a hope about
individuals and becomes a property of the pipeline. The elegant part: **every
guardrail here is also the safety net your refactor already needs.** The gate
that blocks new bad work is the same gate that proves your refactor didn't
regress. You are not building an anti-people apparatus — you're building what a
healthy backend has anyway, and most "people problems" quietly dissolve inside
it.

Install these in roughly this order (each depersonalizes more than the last):

### 1. CI that blocks, not suggests
The single highest-leverage move, because it depersonalizes *everything*: the
pipeline rejects the work, not you. Enforced at the gate, not advisory:
- Tests must pass. Non-negotiable, no merge on red.
- Coverage floor on **new/changed** code (not the whole repo — you'll never
  retrofit legacy to a global number, and trying breeds gaming). "Diff coverage"
  is the tool.
- Linter + formatter (gofmt/golangci-lint). Auto-formatting ends an entire genre
  of review argument.
- Complexity budget on new code (cyclomatic complexity ceiling per function).

### 2. A written Definition of Done
When "done" is explicit — tested, reviewed, meets the SLO, no new p99 regression,
no new lint — "unprofessional" stops being a vibe and becomes a checklist item
that either passed or didn't. A disagreement about a checkbox is a fixable
technical conversation. A disagreement about someone's professionalism is a
fight. Convert every one of the former into the latter's absence.

### 3. Performance gates in CI
Wire Phase 1's load test into the pipeline. A change that regresses p99 past
budget **fails the build** instead of you catching it in prod and pointing
fingers. The number does the accusing; you don't have to. This is also the exact
mechanism that proves a refactor helped — same gate, both jobs.

### 4. Review standards as a shared doc, not your opinion
Write the review bar down and get the team to agree to it *before* you review
anyone's code against it. Now feedback is "this doesn't meet **our** bar" — a
completely different social object than "you're careless." The doc is the
authority; you're just pointing at it. Review the *code*, never the coder; attack
the diff, not the author.

### 5. Observability so the code accuses itself
Tracing + the slow-query log + error dashboards make defects visible to everyone
automatically. The data delivers the verdict in public and impersonally. Nobody
has to be the bearer of bad news, because the dashboard already is.

---

## The escalation path (for the case that doesn't dissolve)

Most people, given a clear bar and a pipeline that enforces it, meet it — that's
why you build the system first. But if someone *actively resists the bar after
it's clear, fair, and agreed*, that's no longer a code problem and no longer
yours to solve alone. Handle it like an incident, not a grievance:

1. **Evidence, not vibes.** The specific pattern, the specific commits, the
   specific impact, and the fact that it persisted after feedback. If you can't
   write it as a pattern-with-impact, you're back in Part 1 — go re-verify.
2. **Private, specific, and about the work.** To your lead, one-on-one. "Here is
   the recurring pattern, here is the effect on the system, here is what I've
   tried." Never a broadside about "the team," never in a public channel, never a
   named callout in a retro.
3. **Hand it off.** Performance management is your lead's job, not yours —
   especially as the new person. Your job was to surface a well-evidenced,
   system-framed observation. Deliver it and let the org act.

The line to hold throughout: you are the senior engineer *raising the floor*, not
the new hire who showed up and started grading colleagues. Every choice in this
file is downstream of that distinction. Build the system, let it enforce the
standard, and reserve the people conversation for the rare, well-evidenced case —
handled privately, by the person whose job it is.
