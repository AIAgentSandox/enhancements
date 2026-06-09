# kepval

`kepval` is a tool that checks whether the YAML metadata in a KEP (Kubernetes
Enhancement Proposal) is valid.

## Getting Started

1. Install `kepval`: `GO111MODULE=on go get k8s.io/enhancements/cmd/kepval`
2. [Optional] clone the enhancements for test data `git clone https://github.com/kubernetes/enhancements.git`
3. Run `kepval <path to kep.md>`

## Development

1. Run the tests with `go test -cover ./...`

## SIG Node assigned reviewers/approvers

A `kep.yaml` may annotate individual entries with the inline comments
`# sig-node-assigned-reviewer` and `# sig-node-assigned-approver`. Any handle so
annotated must also be listed in the `OWNERS` file next to that `kep.yaml`:
assigned reviewers under `reviewers:`, assigned approvers under `approvers:`.
Handles are compared case-insensitively with a leading `@` stripped, so
`@TallClair` in a `kep.yaml` matches `tallclair` in `OWNERS`. The marker must
match exactly, so trailing characters (e.g. a stray quote) cause the annotation
to be ignored.

This is enforced in CI by `TestNodeApprovers` (`test/node_approvers_test.go`),
backed by the `pkg/nodeapprovers` verifier, which runs over the real `keps/`
tree alongside the other Go tests. A mismatch fails the test with the offending
KEP path, role, and user. See
`keps/sig-node/5419-pod-level-resources-in-place-resize/` for an example.
