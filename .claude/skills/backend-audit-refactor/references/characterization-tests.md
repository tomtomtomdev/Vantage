# Characterization Tests — pinning down code you don't understand yet

The premise of a refactor on legacy code is uncomfortable: you must change code
whose behavior you don't fully know, without breaking behavior you don't fully
know. Characterization tests resolve the paradox. You don't write tests for what
the code *should* do — you write tests that document what it *currently* does,
bugs and all, then refactor underneath a green bar. (Michael Feathers, *Working
Effectively with Legacy Code*.)

This is your TDD reflex pointed backwards: instead of red→green→refactor from a
spec, you go observe→pin→refactor from the running system.

## The core workflow

1. **Find a seam.** A seam is a place you can sense behavior without editing the
   code under test — a function boundary, an interface, an HTTP handler, a
   repository method. If there's no seam, your first (separate, behavior-
   preserving) commit is to introduce one: extract the tangle behind an interface
   so it can be called in a test.

2. **Write a test that asserts the wrong thing on purpose.** You often don't know
   the expected output. So assert something obviously false and let the failure
   *tell you* the real value:

   ```go
   got := ComputeFee(order)
   assert.Equal(t, "PLACEHOLDER", got) // deliberately wrong
   // test fails: "expected PLACEHOLDER, got 4275"
   ```
   Now you know the current output is `4275`. Pin it:
   ```go
   assert.Equal(t, 4275, ComputeFee(order))
   ```
   The test now *characterizes* current behavior. Repeat across representative
   inputs — especially the weird branches `git blame` flagged as scars.

3. **Cover the branches, not the lines.** Aim coverage at the code paths you're
   about to touch. Use a coverage tool to confirm the refactor target is actually
   exercised before you change it. Uncovered branch = blind spot = where the
   regression will hide.

4. **Lock in edge cases you don't understand.** If a branch handles some bizarre
   input and you can't tell why (Chesterton's fence), pin its current output
   anyway. The test preserves the scar even if you never learn the story — and
   the day someone tries to "clean it up," the test explains the fence for them.

5. **Refactor under green.** Now change structure freely. If a characterization
   test goes red, you changed behavior — decide deliberately whether that's a fix
   (new separate commit, see below) or an accident (revert).

## Characterization vs. specification tests

| | Characterization | Specification |
|---|---|---|
| Answers | "What does it do *now*?" | "What *should* it do?" |
| Asserts | Observed current output | Intended correct output |
| A pinned bug is | Preserved (on purpose) | A failing test |
| Purpose | Safety net for refactor | Definition of correctness |

A pinned bug is a feature of this technique, not a mistake. You are freezing
behavior so you can move structure. Fixing the bug is a *different* task with its
own commit and its own spec test — never smuggle it into the refactor.

## Golden-master / approval testing (for wide outputs)

When the output is large or messy (a generated report, a serialized payload, a
whole API response), don't hand-assert every field. Capture the full current
output as an approved snapshot ("golden master"), then assert future runs match
it. Any diff is a behavior change surfaced for review. Good for legacy code with
sprawling outputs where enumerating assertions is impractical.

Watch for non-determinism (timestamps, random IDs, map iteration order, floating
point) — scrub or freeze those before snapshotting, or the golden master flaps.

## The commit discipline (this is the load-bearing rule)

- **Refactor commit** = behavior-preserving. Every characterization test stays
  green. Structure changes; observable behavior does not.
- **Bug-fix commit** = behavior-changing, *separate*, and it starts with a
  **failing test that reproduces the bug**, then the fix that turns it green.
  That test becomes a permanent regression guard.

Never in the same commit. A reviewer must be able to look at a diff and know:
"this one is supposed to change nothing" vs. "this one is supposed to change
exactly this." Mixing them makes a regression un-bisectable and a review a guess.

## When you genuinely can't test it

Some code resists all seams (heavy static state, God object, framework-welded).
Options, in order of preference:
1. **Sprout** — write the new behavior in a fresh, tested unit and call into it
   from the untestable code with the smallest possible edit.
2. **Wrap** — put the old thing behind a new tested boundary (this is the seed of
   a strangler-fig migration; see SKILL.md Phase 3).
3. **Pin at a higher level** — if the unit won't yield, characterize at the HTTP
   or integration boundary instead. Coarser, slower, but still a net.

The goal is never 100% coverage of legacy code. It's a net *under the specific
thing you're about to change* — enough to catch you if the refactor slips.
