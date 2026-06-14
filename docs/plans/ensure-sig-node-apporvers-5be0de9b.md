# Ensure SIG Node Approvers

## Overview
Follow up on https://github.com/kubernetes/enhancements/pull/6190/ by adding
verification that every SIG Node KEP (`keps/sig-node/*/kep.yaml`) lists at least
one approver who is either a member of the `sig-node-tech-leads` group (defined in
the repo-root `OWNERS_ALIASES`) or annotated with the inline comment
`# sig-node-assigned-approver`. The rules are stage-dependent:

- If `stage: alpha`, one or more members of `sig-node-tech-leads` MUST be listed
  under `approvers`. Extra non-tech-lead approvers are allowed, but they MUST NOT
  be annotated with `# sig-node-assigned-approver` (the marker is reserved for
  non-alpha stages).
- If `stage` is NOT `alpha`, either a member of `sig-node-tech-leads` OR an
  approver annotated with `# sig-node-assigned-approver` MUST be listed. Extra
  approvers are allowed.

The verification is implemented in the existing `pkg/nodeapprovers` Go package and
wired into CI through the existing `test/node_approvers_test.go` integration test,
mirroring the patterns established in PR #6190.

## Context
- Files involved:
  - Modify: `pkg/nodeapprovers/verify.go` — add `OWNERS_ALIASES` parsing for the
    `sig-node-tech-leads` group and a new `VerifyTechLeadApprovers` verification
    function plus its `Violation`-producing logic.
  - Modify: `pkg/nodeapprovers/verify_test.go` — add unit tests for the new logic.
  - Create: `pkg/nodeapprovers/testdata/techleads/...` fixtures (a small
    `OWNERS_ALIASES` plus several `kep.yaml` fixtures covering valid/invalid cases
    for both alpha and non-alpha stages).
  - Modify: `test/node_approvers_test.go` — add a test that runs the new
    verification over the real `keps/sig-node` tree using the repo-root
    `OWNERS_ALIASES`.
  - Possibly modify: real `keps/sig-node/*/kep.yaml` files that fail the new rule,
    if any exist.
- Related patterns (all from PR #6190):
  - `VerifyKEP` / `VerifyAll` in `pkg/nodeapprovers/verify.go` — YAML node walking
    with `gopkg.in/yaml.v3` to read inline `LineComment` markers, `normalizeUser`
    handle normalization, the `Violation` struct + `String()`, and
    `filepath.WalkDir` over `kep.yaml` files.
  - The `markerOf`, `assignedUsers`, `mappingValue`, `containsNormalized` helpers
    are reused as-is.
  - `test/node_approvers_test.go` — integration test that walks the real `keps/`
    tree and `t.Fatalf`s with a joined list of `Violation.String()` messages.
  - `testdata/` fixture directories each containing a `kep.yaml` (+ optional
    `OWNERS`) consumed by table-driven `require.ElementsMatch` tests.
- Dependencies: `gopkg.in/yaml.v3` and `github.com/stretchr/testify/require`
  (already in `go.mod`). No new dependencies.

## Development Approach
- **Testing approach**: Follow the repository's existing testing practices — add
  table-driven unit tests in `pkg/nodeapprovers/verify_test.go` backed by
  `testdata/` fixtures, and extend the real-tree integration test in
  `test/node_approvers_test.go`.
- Complete each task fully before moving to the next.
- **CRITICAL: all tests must pass before starting next task.**
- Validation commands:
  - `go test ./pkg/nodeapprovers/...`
  - `go test ./test/...` (exercises the real `keps/sig-node` tree)
  - `go vet ./pkg/nodeapprovers/... ./test/...`

## Implementation Steps

### Task 2: Implement tech-lead approver verification logic with unit tests

Add the `sig-node-tech-leads` parsing and the stage-aware approver verification to
`pkg/nodeapprovers/verify.go`, covered by unit tests and fixtures. Do NOT change
the existing `VerifyKEP`/`VerifyAll` behavior; add new exported functions
alongside it.

**Files:**
- Modify: `pkg/nodeapprovers/verify.go`
- Modify: `pkg/nodeapprovers/verify_test.go`
- Create: `pkg/nodeapprovers/testdata/techleads/OWNERS_ALIASES`
- Create: `pkg/nodeapprovers/testdata/techleads/alpha-valid/kep.yaml`
- Create: `pkg/nodeapprovers/testdata/techleads/alpha-missing-techlead/kep.yaml`
- Create: `pkg/nodeapprovers/testdata/techleads/alpha-marker-not-allowed/kep.yaml`
- Create: `pkg/nodeapprovers/testdata/techleads/beta-techlead-valid/kep.yaml`
- Create: `pkg/nodeapprovers/testdata/techleads/beta-marker-valid/kep.yaml`
- Create: `pkg/nodeapprovers/testdata/techleads/beta-missing/kep.yaml`
- Create: `pkg/nodeapprovers/testdata/techleads/no-approvers/kep.yaml`

- [x] Add a constant for the tech-leads alias name, e.g.
  `techLeadsAlias = "sig-node-tech-leads"`, and (reusing the existing
  `approverMarker = "sig-node-assigned-approver"`) keep the marker logic shared.
- [x] Add a function `loadTechLeads(ownersAliasesPath string) (map[string]bool, error)`
  that parses `OWNERS_ALIASES` (YAML shape `aliases: map[string][]string`) and
  returns a set of normalized usernames for the `sig-node-tech-leads` group.
  Return an error if the alias is absent (so misconfiguration is loud).
- [x] Add a helper `approverEntries(kepYAMLPath string) (stage string, entries []approverEntry, err error)`
  that parses the kep.yaml YAML node tree, reads the top-level `stage` scalar, and
  returns the `approvers` sequence as `{User string; Marked bool}` entries, where
  `User` is normalized and `Marked` is true when the entry's `LineComment` matches
  `approverMarker`. Reuse `mappingValue`, `markerOf`, and `normalizeUser`.
- [x] Add `VerifyTechLeadApprovers(kepYAMLPath string, techLeads map[string]bool) ([]Violation, error)`
  implementing the rules. Emit `Violation`s with `Role = approverRole` and clear
  `Reason` strings:
  - No approvers listed → `Reason: "no approvers listed"`.
  - `stage == "alpha"`:
    - No tech-lead among approvers → `Reason: "alpha-stage KEP must list at least one sig-node-tech-leads member as approver"`.
    - Any approver marked `# sig-node-assigned-approver` → one violation per marked
      user with `Reason: "alpha-stage KEP must not use # sig-node-assigned-approver marker"`.
  - `stage != "alpha"`: neither a tech-lead nor a marked approver present →
    `Reason: "non-alpha KEP must list a sig-node-tech-leads member or an approver marked # sig-node-assigned-approver"`.
- [x] Add `VerifyAllTechLeadApprovers(kepsRootDir, ownersAliasesPath string) ([]Violation, error)`
  that loads the tech-leads set once, then `filepath.WalkDir`s `kepsRootDir` for
  `kep.yaml` files (mirroring `VerifyAll`) and aggregates violations.
- [x] Create the `testdata/techleads/OWNERS_ALIASES` fixture defining
  `sig-node-tech-leads` with a couple of handles (e.g. `dchen1107`, `mrunalp`).
- [x] Create the per-case `kep.yaml` fixtures listed above covering: alpha with a
  tech lead (valid), alpha missing a tech lead (violation), alpha with a tech lead
  but an extra approver wrongly marked `# sig-node-assigned-approver` (violation),
  beta with a tech lead (valid), beta with a non-tech-lead marked approver (valid),
  beta with neither (violation), and a KEP with an empty/absent approvers list
  (violation).
- [x] Add table-driven unit tests in `verify_test.go` (`TestVerifyTechLeadApprovers`
  and `TestVerifyAllTechLeadApprovers`) using `require.ElementsMatch`, following the
  existing `violationsFor` helper pattern (add an analogous helper if the fixture
  layout differs).
- [x] Run `go test ./pkg/nodeapprovers/...` and `go vet ./pkg/nodeapprovers/...`;
  fix until green.
- [x] Commit with message: `feat: add sig-node tech-lead approver verification`

### Task 3: Wire verification into CI integration test over the real keps tree

Run the new verification over the real `keps/sig-node` tree with the repo-root
`OWNERS_ALIASES`, and resolve any real violations so the test passes.

**Files:**
- Modify: `test/node_approvers_test.go`
- Possibly modify: offending `keps/sig-node/*/kep.yaml` files (only if real
  violations exist)

- [x] Add `TestNodeTechLeadApprovers` to `test/node_approvers_test.go` that locates
  the repo root (parent of the test working directory, as the existing test does),
  calls `nodeapprovers.VerifyAllTechLeadApprovers(filepath.Join(rootDir, "keps", "sig-node"), filepath.Join(rootDir, "OWNERS_ALIASES"))`,
  and `t.Fatalf`s with the joined `Violation.String()` messages when violations
  are found (mirror the existing `TestNodeApprovers` structure).
- [x] Run `go test ./test/...` to surface any real violations in the live
  `keps/sig-node` tree.
- [x] For each real violation found, fix the offending `kep.yaml` by ensuring an
  appropriate approver is present per the stage rules: for non-alpha KEPs add the
  `# sig-node-assigned-approver` marker to an already-listed SIG Node approver (or
  add a tech lead); for alpha KEPs ensure a `sig-node-tech-leads` member is listed
  and remove any disallowed `# sig-node-assigned-approver` markers. Make the
  minimal change that satisfies the rule; do not invent approvers — prefer marking
  an existing legitimate approver. If a violation cannot be resolved by a mechanical
  edit (e.g. no eligible approver is listed at all), note it in the commit message.
  - Resolved by: (1) teaching `VerifyTechLeadApprovers` to accept the
    `@sig-node-tech-leads` group alias listed directly under `approvers` as
    satisfying the tech-lead requirement (this was an integration-test-discovered
    gap; many KEPs list the alias rather than an individual member); (2) adding
    `@sig-node-tech-leads` as an approver to non-alpha KEPs that lacked any tech
    lead or marked approver (marking an existing approver was not viable since most
    of those dirs have no neighboring OWNERS file, which the existing
    `TestNodeApprovers` requires for marked approvers); (3) replacing the `TBD`
    approver in alpha `4216-image-pull-per-runtime-class` with the tech-leads
    alias; and (4) for alpha KEPs `5526`, `5607`, `5825` that prematurely used the
    `# sig-node-assigned-approver` marker, removing the marker from `kep.yaml` and
    the corresponding assigned-approver entry from the neighboring `OWNERS` file
    (leaving an empty `approvers:` list, matching existing fixtures like
    `2570-memory-qos`); the affected people remain listed as reviewers and can be
    re-added as assigned approvers when the KEP reaches beta.
- [x] Re-run `go test ./test/...` and `go test ./pkg/nodeapprovers/...` until both
  pass; run `go vet ./test/... ./pkg/nodeapprovers/...`.
- [x] Commit with message: `feat: verify sig-node tech-lead approvers in CI`
