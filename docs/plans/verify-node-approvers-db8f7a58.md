# Verify SIG Node Assigned Reviewers/Approvers

## Overview

Add Go verification logic to the `kubernetes/enhancements` repository that scans every
`kep.yaml` file for reviewers/approvers annotated with the inline YAML comments
`# sig-node-assigned-reviewer` and `# sig-node-assigned-approver`, and ensures each such
person is listed in the `OWNERS` file sitting next to that `kep.yaml`
(assigned-reviewers must appear under `reviewers:` in OWNERS, assigned-approvers under
`approvers:`). The comment markers are hardcoded for now. The check is wired into CI so it
runs automatically with the existing Go test suite.

Reference example:
- `keps/sig-node/5419-pod-level-resources-in-place-resize/kep.yaml` has
  `- "@tallclair" # sig-node-assigned-reviewer` and
  `- "@tallclair" # sig-node-assigned-approver`.
- `keps/sig-node/5419-pod-level-resources-in-place-resize/OWNERS` lists `tallclair` under
  both `reviewers:` and `approvers:`.

## Context

- Files involved:
  - Create: `pkg/nodeapprovers/verify.go` — core parsing + verification logic.
  - Create: `pkg/nodeapprovers/verify_test.go` — unit tests over local testdata.
  - Create: `pkg/nodeapprovers/testdata/...` — small fixture KEP dirs (valid + invalid).
  - Create: `test/node_approvers_test.go` — integration test that runs the verifier over
    the real `keps/` tree (mirrors `test/metadata_test.go`); this is what plugs the check
    into CI.
  - Reference only: `keps/sig-node/5419-pod-level-resources-in-place-resize/kep.yaml` and
    its `OWNERS` (the canonical example), `hack/test-go.sh`, `Makefile`.
- Related patterns:
  - `test/metadata_test.go` is the existing pattern for a repo-wide validation test: it
    gets the repo root via `os.Getwd()`/`filepath.Dir`, calls into a `pkg/` function, and
    asserts no errors with `github.com/stretchr/testify/require`. The new integration test
    follows this exact shape.
  - YAML is parsed with `gopkg.in/yaml.v3` (already a dependency, see `pkg/yaml/yaml.go`).
    Inline comments are NOT available through plain struct unmarshalling — they require the
    `yaml.Node` API, where each scalar's `LineComment` field holds text like
    `# sig-node-assigned-reviewer`. The verifier must decode the file into a `yaml.Node`
    tree and walk the `reviewers`/`approvers` sequence nodes to read each item's
    `LineComment`.
  - OWNERS files are plain YAML with `reviewers:` / `approvers:` string lists and can be
    decoded with a simple struct. Names in OWNERS are bare (`tallclair`) while `kep.yaml`
    uses `@tallclair`; normalize by trimming a leading `@` and lowercasing (GitHub handles
    are case-insensitive).
  - Every Go file starts with the Apache boilerplate header from
    `hack/boilerplate/boilerplate.go.txt` (the `goheader` linter in `.golangci.yml`
    enforces it). Copy it verbatim with the current year.
- Dependencies: none new. Uses stdlib (`os`, `path/filepath`, `strings`, `io/fs`),
  `gopkg.in/yaml.v3`, and `github.com/stretchr/testify/require` (test only) — all already
  in `go.mod`.

## Development Approach

- **Testing approach**: Follow the repository's existing testing practices — table-driven
  Go unit tests in the package (`pkg/nodeapprovers/verify_test.go`) using local
  `testdata/` fixtures, plus a repo-wide integration test under `test/` mirroring
  `test/metadata_test.go`. Tests are run by `hack/test-go.sh` (`make test-go-unit`), which
  is already part of CI via `go list ./...`.
- Complete each task fully before moving to the next.
- **CRITICAL: all tests must pass before starting next task.**
- Validation commands (run from repo root):
  - `go build ./...`
  - `go test ./pkg/nodeapprovers/...` (Task 1)
  - `go test ./pkg/nodeapprovers/... ./test/...` (Task 2)
  - `gofmt -l pkg/nodeapprovers test/node_approvers_test.go` (must print nothing)
  - If available: `make verify-golangci-lint` (golangci-lint) — otherwise rely on
    `go vet ./pkg/nodeapprovers/... ./test/...`.

## Implementation Steps

### Task 1: Core verification package with unit tests

Create the `nodeapprovers` package that parses a `kep.yaml`'s assigned reviewers/approvers
from inline YAML comments, parses the adjacent `OWNERS` file, and reports any assigned
person that is missing from the correct OWNERS list. Provide a directory-walking entry
point and cover it with table-driven unit tests over local fixtures.

**Files:**
- Create: `pkg/nodeapprovers/verify.go`
- Create: `pkg/nodeapprovers/verify_test.go`
- Create: `pkg/nodeapprovers/testdata/` fixtures (see below)

**Design details:**
- Hardcoded markers (package-level constants):
  - `reviewerMarker = "sig-node-assigned-reviewer"`
  - `approverMarker = "sig-node-assigned-approver"`
  - Match by checking the trimmed `LineComment` (strip leading `#` and spaces) equals the
    marker, so a comment like `# sig-node-tl` on another line does not match.
- Public API (suggested):
  - `type Violation struct { KEPPath string; Role string; User string; Reason string }`
    with a `String()` method for readable output.
  - `func VerifyKEP(kepYAMLPath string) ([]Violation, error)` — verify one `kep.yaml`.
  - `func VerifyAll(rootDir string) ([]Violation, error)` — walk `rootDir` (typically the
    repo's `keps/` directory) for files named `kep.yaml` and aggregate violations.
- `VerifyKEP` logic:
  1. Read the file; `yaml.Unmarshal` into a `yaml.Node`. Get the document's mapping node.
  2. For the `reviewers` and `approvers` mapping values (sequence nodes), iterate items;
     for each scalar item whose `LineComment` matches the role's marker, record the
     normalized username (trim leading `@`, lowercase).
  3. If there are no assigned entries, return no violations (and do not require an OWNERS
     file).
  4. Resolve the OWNERS path as `filepath.Join(filepath.Dir(kepYAMLPath), "OWNERS")`.
     If assigned entries exist but OWNERS is missing/unreadable, emit a violation
     (Reason: "OWNERS file not found").
  5. Decode OWNERS into `struct { Reviewers []string; Approvers []string }` (normalize
     names the same way). For each assigned-reviewer not in OWNERS reviewers → violation;
     each assigned-approver not in OWNERS approvers → violation.
- Be tolerant of `kep.yaml` files that have no `reviewers`/`approvers` keys (skip).

**Testdata fixtures to create (minimal `kep.yaml` + `OWNERS` per dir):**
- `testdata/valid/kep.yaml` — assigned reviewer & approver both present in
  `testdata/valid/OWNERS`. Expect 0 violations.
- `testdata/missing-reviewer/kep.yaml` — assigned reviewer NOT in `OWNERS reviewers`.
  Expect 1 violation.
- `testdata/missing-approver/kep.yaml` — assigned approver NOT in `OWNERS approvers`.
  Expect 1 violation.
- `testdata/no-owners/kep.yaml` — has an assigned reviewer but no `OWNERS` file in the
  dir. Expect 1 violation (OWNERS not found).
- `testdata/no-markers/kep.yaml` — reviewers/approvers present but none carry the marker
  comments (and no OWNERS). Expect 0 violations.

- [ ] Implement `pkg/nodeapprovers/verify.go` with the markers, `Violation` type,
      `VerifyKEP`, and `VerifyAll` as described, including the Apache boilerplate header.
- [ ] Create the five `testdata/` fixture directories with `kep.yaml` (and `OWNERS` where
      applicable) matching the example comment style from KEP 5419.
- [ ] Write table-driven unit tests in `pkg/nodeapprovers/verify_test.go` covering each
      fixture (assert expected violation counts and that the right user/role is reported),
      plus a direct `VerifyAll(testdata)` aggregation test.
- [ ] Run `gofmt -l`, `go vet`, `go build ./...`, and `go test ./pkg/nodeapprovers/...`;
      fix until all pass.

### Task 2: Wire the verifier into CI via a repo-wide integration test

Add an integration test, modeled on `test/metadata_test.go`, that runs `VerifyAll` over
the repository's real `keps/` directory and fails if any assigned reviewer/approver is
missing from the neighboring OWNERS file. Because the test lives under `test/` and is
picked up by `go list ./...`, it runs automatically in CI through `hack/test-go.sh`
(`make test-go-unit`) — no test-infra/Prow changes are required in this repo.

**Files:**
- Create: `test/node_approvers_test.go`

**Design details:**
- Package `test` (same as `metadata_test.go`).
- Resolve the repo root via `os.Getwd()` + `filepath.Dir` (the test runs from `test/`),
  then call `nodeapprovers.VerifyAll(filepath.Join(rootDir, "keps"))`.
- `require.NoError(t, err)`; if `len(violations) > 0`, fail with a readable message listing
  each violation (so a contributor sees exactly which KEP/user/role is wrong).
- This test will pass today because KEP 5419's OWNERS already lists `tallclair` under both
  roles; confirm by running it against the live tree.

- [ ] Implement `test/node_approvers_test.go` calling `nodeapprovers.VerifyAll` over the
      real `keps/` directory and asserting zero violations, with the Apache boilerplate
      header.
- [ ] Run `go test ./test/...` (and `go test ./pkg/nodeapprovers/... ./test/...`) and
      confirm it passes against the current repository contents.
- [ ] Run `gofmt -l` and `go vet` on the new test; confirm `make test-go-unit` (or the
      equivalent `hack/test-go.sh`) exercises the new test so CI coverage is in place.
