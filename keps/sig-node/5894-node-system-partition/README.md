# KEP-5894: Node System Partition

<!-- toc -->
- [Release Signoff Checklist](#release-signoff-checklist)
- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
- [Proposal](#proposal)
  - [User Stories (Optional)](#user-stories-optional)
    - [Story 1: System Daemon Isolation](#story-1-system-daemon-isolation)
    - [Story 2: HPC Workloads](#story-2-hpc-workloads)
  - [Notes/Constraints/Caveats (Optional)](#notesconstraintscaveats-optional)
  - [Risks and Mitigations](#risks-and-mitigations)
- [Design Details](#design-details)
  - [Cgroup Hierarchy](#cgroup-hierarchy)
    - [Creation and Hosting](#creation-and-hosting)
    - [Resource Limiting](#resource-limiting)
  - [Configuration](#configuration)
  - [Eviction](#eviction)
  - [Test Plan](#test-plan)
      - [Prerequisite testing updates](#prerequisite-testing-updates)
      - [Unit tests](#unit-tests)
      - [Integration tests](#integration-tests)
      - [e2e tests](#e2e-tests)
  - [Graduation Criteria](#graduation-criteria)
    - [Alpha](#alpha)
  - [Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy)
  - [Version Skew Strategy](#version-skew-strategy)
- [Production Readiness Review Questionnaire](#production-readiness-review-questionnaire)
  - [Feature Enablement and Rollback](#feature-enablement-and-rollback)
  - [Rollout, Upgrade and Rollback Planning](#rollout-upgrade-and-rollback-planning)
  - [Monitoring Requirements](#monitoring-requirements)
  - [Dependencies](#dependencies)
  - [Scalability](#scalability)
  - [Troubleshooting](#troubleshooting)
- [Implementation History](#implementation-history)
- [Drawbacks](#drawbacks)
- [Alternatives](#alternatives)
- [Infrastructure Needed (Optional)](#infrastructure-needed-optional)
<!-- /toc -->

## Release Signoff Checklist

Items marked with (R) are required *prior to targeting to a milestone / release*.

- [X] (R) Enhancement issue in release milestone, which links to KEP dir in [kubernetes/enhancements] (not the initial KEP PR)
- [ ] (R) KEP approvers have approved the KEP status as `implementable`
- [ ] (R) Design details are appropriately documented
- [ ] (R) Test plan is in place, giving consideration to SIG Architecture and SIG Testing input (including test refactors)
  - [ ] e2e Tests for all Beta API Operations (endpoints)
  - [ ] (R) Ensure GA e2e tests meet requirements for [Conformance Tests](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/conformance-tests.md)
  - [ ] (R) Minimum Two Week Window for GA e2e tests to prove flake free
- [ ] (R) Graduation criteria is in place
  - [ ] (R) [all GA Endpoints](https://github.com/kubernetes/community/pull/1806) must be hit by [Conformance Tests](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/conformance-tests.md) within one minor version of promotion to GA
- [ ] (R) Production readiness review completed
- [ ] (R) Production readiness review approved
- [ ] "Implementation History" section is up-to-date for milestone
- [ ] User-facing documentation has been created in [kubernetes/website], for publication to [kubernetes.io]
- [ ] Supporting documentation—e.g., additional design documents, links to mailing list discussions/SIG meetings, relevant PRs/issues, release notes

<!--
**Note:** This checklist is iterative and should be reviewed and updated every time this enhancement is being considered for a milestone.
-->

[kubernetes.io]: https://kubernetes.io/
[kubernetes/enhancements]: https://git.k8s.io/enhancements
[kubernetes/kubernetes]: https://git.k8s.io/kubernetes
[kubernetes/website]: https://git.k8s.io/website

## Summary

Node System Partition introduces a dedicated partition on a Node for
system Pods (e.g., kube-system workloads), isolating them from user
workloads. The system partition has its own cgroup hierarchy with
dedicated CPU set and memory limits, ensuring system Pods cannot
interfere with user Pods and vice versa.

There are a few DIY solutions for system daemon isolation, but this
KEP is needed because enforcing memory limits requires a separate
cgroup hierarchy, and integrating that with kubelet functions like
metrics collection and eviction is impossible to implement as a
plugin.

This KEP is scoped to a single system partition. Supporting arbitrary
user-defined partitions is a non-goal.

## Motivation

Isolating system daemonsets from user workloads is a longstanding
problem with numerous DIY solutions and no obvious winner. Today,
system Pods and user Pods share the same resource boundaries — system
Pods can burst into user resources and vice versa. This makes it
impossible to guarantee that critical system components have the
resources they need, or that user workloads are free from system
interference.

This problem is increasingly important as Kubernetes targets new
workload types:

1. **Traditional workloads**: Benefit from overcommit, but need basic
   separation so a misbehaving system daemon doesn’t destabilize user
   Pods.
2. **HPC workloads**: Require minimal system interference and strict
   resource isolation. These workloads need system components
   constrained to a small, bounded resource footprint.
3. **AI/ML workloads**: Use specialized devices and need a responsive
   management layer that is sandboxed and guaranteed its own
   resources, without competing with the user workload.

A dedicated system partition solves these problems by giving system
Pods their own resource-limited cgroup hierarchy, eliminating
interference between the management layer and user workloads.

### Goals

Alpha stage:

- Introduce a system partition with a dedicated cgroup hierarchy for
  system Pods (e.g., kube-system namespace).
- Support memory limiting via the system partition's cgroup root.
- Support setting a dedicated CPU set for system partition Pods.
- Kubelet treats system and default partitions independently for
  resource allocation and overcommit logic.
- System partition is statically defined via kubelet configuration.
- System partition shares resources with kubelet, container runtime,
  and other host processes.

After alpha:

- Scheduling integration to target Pods to the system partition.
- Additional resource isolation between system and default partitions.

### Non-Goals

- Supporting multiple arbitrary partitions. This KEP is scoped to a single system partition only.
- Implementing this isolation purely via external plugins (NRI, DRA) without kubelet changes, as metrics collection and eviction require deep integration.
- Changing the fundamental QoS levels or resource accounting logic within a partition.

## Proposal

The Node System Partition introduces a system partition — a
resource-bounded area of the Node dedicated to running system Pods
(e.g., kube-system workloads). The system partition has dedicated
CPUs, memory limits, and its own cgroup hierarchy. Pods in the system
partition follow the same QoS levels as they would on a Node today,
but are constrained to the partition's resources. All overcommit
within the system partition happens against its resource budget only.

Kubelet treats the system partition and the default (user) partition
independently for resource allocation and overcommit, using the same
logic currently defined for the whole Node. This avoids race
conditions or double-accounting that can occur with external
management approaches like NRI or DRA.

The default partition retains the existing cgroup hierarchy as-is.
Only system Pods are moved to a new sub-hierarchy under `kubepods`.
This minimizes impact on external monitoring tools, container
runtimes, and other node-level agents that rely on the standard
Kubernetes cgroup layout.

In the alpha stage, Node Allocatable will not change — the KEP
assumes the administrator has correctly accounted for system Pod
resources. In later stages, the Node may report separate allocatable
values for the system partition.

### User Stories (Optional)

<!--
Detail the things that people will be able to do if this KEP is implemented.
Include as much detail as possible so that people can understand the "how" of
the system. The goal here is to make this feel real for users without getting
bogged down.
-->

#### Story 1: System Daemon Isolation

As a cluster administrator, I want to sandbox system daemonsets into a dedicated partition so that they do not interfere with user workloads and have a guaranteed amount of resources (CPU, Memory) regardless of user Pod activity.

#### Story 2: HPC Workloads

As an HPC user, I want to run my performance-critical applications in a partition consisting of high-performance cores with dedicated memory. This ensures my workloads have uninterrupted performance and eliminates noisy neighbor issues from system Pods.

### Notes/Constraints/Caveats (Optional)

<!--
What are the caveats to the proposal?
What are some important details that didn't come across above?
Go in to as much detail as necessary here.
This might be a good place to talk about core concepts and how they relate.
-->

The KEP will limit the scope to cgroup v2. Alpha stage is limited to understand the problems that a separate Node partition can cause on a Node and resolve those problems.

### Risks and Mitigations

<!--
What are the risks of this proposal, and how do we mitigate? Think broadly.
For example, consider both security and how this will impact the larger
Kubernetes ecosystem.

How will security be reviewed, and by whom?

How will UX be reviewed, and by whom?

Consider including folks who also work outside the SIG or subproject.
-->

- **Unexpected Evictions**: Since memory limits are enforced at the partition root, Pods that were previously able to consume unused node memory may now be evicted when the partition limit is reached. Mitigation: Clear documentation and monitoring for partition-level resource usage.
- **Kubelet Complexity**: Adding separate cgroup hierarchies and partition-aware eviction logic increases Kubelet complexity. Mitigation: Extensive unit and integration testing of the new `container manager` logic.
- **External tools integration**: Various external tools may be confused by the updated cgroup hierarchy. The KEP will explore potential issues and mitigations.

## Design Details

<!--
This section should contain enough information that the specifics of your
change are understandable. This may include API specs (though not always
required) or even code snippets. If there's any ambiguity about HOW your
proposal will be implemented, this is the place to discuss them.
-->

The recommendation is to make Node Partitions as a dedicated CPU set as well as a separate cgroup hierarchy so the memory limit and other properties can be applied to the whole partition.

### Cgroup Hierarchy

To enforce resource isolation at the partition level while adhering to the **Minimal Impact** principle, the Kubelet will introduce a targeted sub-hierarchy for system workloads while preserving the legacy structure for user workloads.

**Existing Hierarchy:**
`kubepods` -> `burstable` / `besteffort` / `pod<UID>`

**Proposed Hierarchy:**
`kubepods`
├── `system` (New partition root for system workloads)
│   ├── `burstable`
│   ├── `besteffort`
│   └── `pod<UID>`
├── `burstable` (Legacy location, remains for non-system workloads)
├── `besteffort` (Legacy location, remains for non-system workloads)
└── `pod<UID>` (Legacy location, remains for non-system workloads)

#### Creation and Hosting

- **Pod Placement**: During Pod admission and sync, the Kubelet will determine if a Pod belongs to the `system` partition (e.g., via namespace check or explicit configuration). If it does, the sandbox and containers will be placed under the `kubepods/system` hierarchy. All other Pods will continue to use the legacy paths directly under `kubepods`.
- **QoS Management**: The `qos_container_manager` will be updated to manage QoS cgroups in both the legacy location and the new `system` partition. It will reconcile and maintain `burstable` and `besteffort` roots under `kubepods/system` separately from the legacy roots.

#### Resource Limiting

Initially, the separate hierarchy will be used to set hard memory limits for the `system` partition.

For the "user" workload (the legacy hierarchy), resource isolation is effectively maintained by the fact that the Kubelet subtracts the `system` partition's resources from the node's total allocatable capacity. Since user Pods are scheduled against this reduced allocatable capacity, they are naturally constrained within their intended boundaries without needing a separate nested cgroup root.

### Configuration

Kubelet will have a configuration section to describe the system partition. It will include memory allocatable size and limits as well as a set of CPUs that needs to be associated with the partition.

### Eviction

The Kubelet's eviction manager will be updated to enforce partition-level resource boundaries, specifically for non-compressible resources like memory.

- **Partition Usage Monitoring**: Kubelet will monitor the aggregate resource usage of each partition root cgroup. This can be achieved by summing up metrics from the `summaryProvider` or directly reading cgroup stats (e.g., `memory.current`).
- **Targeted Eviction**: When a partition's memory usage exceeds its configured limit, the eviction manager will target Pods *within that specific partition*. This prevents a "noisy neighbor" in the `user` partition from causing the eviction of critical Pods in the `system` partition.
- **Ranking**: Within a partition, Pods will be ranked for eviction based on existing criteria (QoS class, priority, and resource usage relative to requests).

### Test Plan

<!--
**Note:** *Not required until targeted at a release.*
The goal is to ensure that we don't accept enhancements with inadequate testing.

All code is expected to have adequate tests (eventually with coverage
expectations). Please adhere to the [Kubernetes testing guidelines][testing-guidelines]
when drafting this test plan.

[testing-guidelines]: https://git.k8s.io/community/contributors/devel/sig-testing/testing.md
-->

[ ] I/we understand the owners of the involved components may require updates to
existing tests to make this code solid enough prior to committing the changes necessary
to implement this enhancement.

##### Prerequisite testing updates

<!--
Based on reviewers feedback describe what additional tests need to be added prior
implementing this enhancement to ensure the enhancements have also solid foundations.
-->

##### Unit tests

<!--
In principle every added code should have complete unit test coverage, so providing
the exact set of tests will not bring additional value.
However, if complete unit test coverage is not possible, explain the reason of it
together with explanation why this is acceptable.
-->

<!--
Additionally, for Alpha try to enumerate the core package you will be touching
to implement this enhancement and provide the current unit coverage for those
in the form of:
- <package>: <date> - <current test coverage>
The data can be easily read from:
https://testgrid.k8s.io/sig-testing-canaries#ci-kubernetes-coverage-unit

This can inform certain test coverage improvements that we want to do before
extending the production code to implement this enhancement.
-->

- `<package>`: `<date>` - `<test coverage>`

##### Integration tests

<!--
Integration tests are contained in https://git.k8s.io/kubernetes/test/integration.
Integration tests allow control of the configuration parameters used to start the binaries under test.
This is different from e2e tests which do not allow configuration of parameters.
Doing this allows testing non-default options and multiple different and potentially conflicting command line options.
For more details, see https://github.com/kubernetes/community/blob/master/contributors/devel/sig-testing/testing-strategy.md

If integration tests are not necessary or useful, explain why.
-->

<!--
This question should be filled when targeting a release.
For Alpha, describe what tests will be added to ensure proper quality of the enhancement.

For Beta and GA, document that tests have been written,
have been executed regularly, and have been stable.
This can be done with:
- permalinks to the GitHub source code
- links to the periodic job (typically https://testgrid.k8s.io/sig-release-master-blocking#integration-master), filtered by the test name
- a search in the Kubernetes bug triage tool (https://storage.googleapis.com/k8s-triage/index.html)
-->

- [test name](https://github.com/kubernetes/kubernetes/blob/2334b8469e1983c525c0c6382125710093a25883/test/integration/...): [integration master](https://testgrid.k8s.io/sig-release-master-blocking#integration-master?include-filter-by-regex=MyCoolFeature), [triage search](https://storage.googleapis.com/k8s-triage/index.html?test=MyCoolFeature)

##### e2e tests

<!--
This question should be filled when targeting a release.
For Alpha, describe what tests will be added to ensure proper quality of the enhancement.

For Beta and GA, document that tests have been written,
have been executed regularly, and have been stable.
This can be done with:
- permalinks to the GitHub source code
- links to the periodic job (typically a job owned by the SIG responsible for the feature), filtered by the test name
- a search in the Kubernetes bug triage tool (https://storage.googleapis.com/k8s-triage/index.html)

We expect no non-infra related flakes in the last month as a GA graduation criteria.
If e2e tests are not necessary or useful, explain why.
-->

- [test name](https://github.com/kubernetes/kubernetes/blob/2334b8469e1983c525c0c6382125710093a25883/test/e2e/...): [SIG ...](https://testgrid.k8s.io/sig-...?include-filter-by-regex=MyCoolFeature), [triage search](https://storage.googleapis.com/k8s-triage/index.html?test=MyCoolFeature)

### Graduation Criteria

<!--
**Note:** *Not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, [feature gate] graduations, or as
something else. The KEP should keep this high-level with a focus on what
signals will be looked at to determine graduation.

Consider the following in developing the graduation criteria for this enhancement:
- [Maturity levels (`alpha`, `beta`, `stable`)][maturity-levels]
- [Feature gate][feature gate] lifecycle
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc
definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning)
or by redefining what graduation means.

In general we try to use the same stages (alpha, beta, GA), regardless of how the
functionality is accessed.

[feature gate]: https://git.k8s.io/community/contributors/devel/sig-architecture/feature-gates.md
[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

Below are some examples to consider, in addition to the aforementioned [maturity levels][maturity-levels].

#### Alpha

- Feature implemented behind a feature flag
- Initial e2e tests completed and enabled

#### Beta

- Gather feedback from developers and surveys
- Complete features A, B, C
- Additional tests are in Testgrid and linked in KEP
- More rigorous forms of testing—e.g., downgrade tests and scalability tests
- All functionality completed
- All security enforcement completed
- All monitoring requirements completed
- All testing requirements completed
- All known pre-release issues and gaps resolved

**Note:** Beta criteria must include all functional, security, monitoring, and testing requirements along with resolving all issues and gaps identified

#### GA

- N examples of real-world usage
- N installs
- Allowing time for feedback
- All issues and gaps identified as feedback during beta are resolved

**Note:** GA criteria must not include any functional, security, monitoring, or testing requirements.  Those must be beta requirements.

**Note:** Generally we also wait at least two releases between beta and
GA/stable, because there's no opportunity for user feedback, or even bug reports,
in back-to-back releases.

**For non-optional features moving to GA, the graduation criteria must include
[conformance tests].**

[conformance tests]: https://git.k8s.io/community/contributors/devel/sig-architecture/conformance-tests.md

#### Deprecation

<!--
- Announce deprecation and support policy of the existing flag
- Two versions passed since introducing the functionality that deprecates the flag (to address version skew)
- Address feedback on usage/changed behavior, provided on GitHub issues
- Deprecate the flag
-->

#### Alpha

- Kubelet can be configured to host a "system" partition and schedule system Pods to this partition.
- System partition is tested with Containerd and/or CRI-O with the version of container runtime exist that supports the new cgroup hierarchy.
- Metrics are collected correctly for system Pods.
- Node e2e tests are validating the new functionality.

### Upgrade / Downgrade Strategy

<!--
If applicable, how will the component be upgraded and downgraded? Make sure
this is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade, in order to maintain previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade, in order to make use of the enhancement?
-->

### Version Skew Strategy

<!--
If applicable, how will the component handle version skew with other
components? What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- Does this enhancement involve coordinating behavior in the control plane and nodes?
- How does an n-3 kubelet or kube-proxy without this feature available behave when this feature is used?
- How does an n-1 kube-controller-manager or kube-scheduler without this feature available behave when this feature is used?
- Will any other components on the node change? For example, changes to CSI,
  CRI or CNI may require updating that component before the kubelet.
-->

## Production Readiness Review Questionnaire

### Feature Enablement and Rollback

###### How can this feature be enabled / disabled in a live cluster?

- [x] Feature gate (also fill in values in `kep.yaml`)
  - Feature gate name: `NodeSystemPartition`
  - Components depending on the feature gate: `kubelet`

###### Does enabling the feature change any default behavior?

No. The feature requires both the feature gate to be enabled and
explicit kubelet configuration defining the system partition. Without
configuration, kubelet behaves identically to today.

###### Can the feature be disabled once it has been enabled (i.e. can we roll back the enablement)?

Yes. Disable the feature gate and restart kubelet. On restart,
kubelet will not create or manage the system partition cgroup
hierarchy. Existing system Pods will be restarted in their default
cgroup locations under `kubepods`. The orphaned `kubepods/system/`
cgroup hierarchy will be cleaned up by kubelet's cgroup garbage
collection.

###### What happens if we reenable the feature if it was previously rolled back?

Kubelet will recreate the system partition cgroup hierarchy and, on
the next Pod sync, move system Pods into the `kubepods/system/`
hierarchy. This involves container restarts for affected Pods.

###### Are there any tests for feature enablement/disablement?

Unit tests will verify that the container manager correctly creates
or skips the system partition cgroup hierarchy based on the feature
gate state. Node e2e tests will verify system Pods are placed in the
correct cgroup location with the feature enabled and disabled.

### Rollout, Upgrade and Rollback Planning

###### How can a rollout or rollback fail? Can it impact already running workloads?

This is a node-local feature with no control plane component. Rollout
is per-node via kubelet restart. If the system partition
configuration is invalid (e.g., references CPUs that don't exist),
kubelet will fail to start. Running user workloads are not affected
by enabling the feature since they remain in the default cgroup
locations.

###### What specific metrics should inform a rollback?

- Unexpected Pod evictions in the system partition (system Pods being
  OOM-killed due to undersized memory limit).
- Kubelet restart failures.
- System Pod startup latency increases.

###### Were upgrade and rollback tested? Was the upgrade->downgrade->upgrade path tested?

Will be tested manually before alpha release. The upgrade path
involves kubelet restart with the feature gate enabled and system
partition configured. The downgrade path involves disabling the
feature gate and restarting kubelet.

###### Is the rollout accompanied by any deprecations and/or removals of features, APIs, fields of API types, flags, etc.?

No.

### Monitoring Requirements

###### How can an operator determine if the feature is in use by workloads?

Check if the `kubepods/system` cgroup hierarchy exists on the node.
A kubelet metric will be added to indicate whether the system
partition is configured and active.

###### How can someone using this feature know that it is working for their instance?

- [x] Other (treat as last resort)
  - Details: Verify that system Pods (e.g., kube-system Pods) are
    placed under the `kubepods/system/` cgroup hierarchy by
    inspecting `/sys/fs/cgroup/kubepods/system/`. Verify memory
    limits are set by reading `memory.max` on the system partition
    cgroup.

###### What are the reasonable SLOs (Service Level Objectives) for the enhancement?

No new SLOs. The feature should not degrade existing Pod startup
latency SLOs. System Pods should start with the same latency as
before, within the system partition's resource constraints.

###### What are the SLIs (Service Level Indicators) an operator can use to determine the health of the service?

- [x] Metrics
  - Metric name: `kubelet_system_partition_memory_usage_bytes`
  - Components exposing the metric: kubelet
- [x] Metrics
  - Metric name: `kubelet_system_partition_memory_limit_bytes`
  - Components exposing the metric: kubelet

###### Are there any missing metrics that would be useful to have to improve observability of this feature?

Per-partition eviction counts would be useful but will be deferred
to beta to limit alpha scope.

### Dependencies

###### Does this feature depend on any specific services running in the cluster?

- Container runtime (containerd or CRI-O)
  - Usage description: The container runtime must support placing Pod
    sandboxes under a non-default cgroup parent via the CRI
    `CgroupParent` field.
  - Impact of its outage on the feature: Pods cannot be created in
    the system partition.
  - Impact of its degraded performance or high-error rates on the
    feature: No additional impact beyond normal Pod creation failures.

### Scalability

###### Will enabling / using this feature result in any new API calls?

No. This is a node-local feature. Kubelet does not make additional
API calls when the system partition is configured.

###### Will enabling / using this feature result in introducing new API types?

No.

###### Will enabling / using this feature result in any new calls to the cloud provider?

No.

###### Will enabling / using this feature result in increasing size or count of the existing API objects?

No.

###### Will enabling / using this feature result in increasing time taken by any operations covered by existing SLIs/SLOs?

Negligible. Pod admission will include an additional check to
determine whether a Pod belongs to the system partition. This is a
simple namespace or label check.

###### Will enabling / using this feature result in non-negligible increase of resource usage (CPU, RAM, disk, IO, ...) in any components?

Minimal. Kubelet will maintain additional in-memory state for the
system partition cgroup (resource limits, usage stats). This is a
small constant overhead — one additional cgroup hierarchy to monitor.

###### Can enabling / using this feature result in resource exhaustion of some node resources (PIDs, sockets, inodes, etc.)?

No. The feature creates a small number of additional cgroup
directories (system partition root + QoS sub-cgroups). This is
bounded and constant regardless of Pod count.

### Troubleshooting

###### How does this feature react if the API server and/or etcd is unavailable?

No impact. The system partition is configured locally via kubelet
config and enforced via cgroups. It does not depend on API server
availability. Already-running Pods in the system partition continue
to operate normally.

###### What are other known failure modes?

- System partition memory limit too low
  - Detection: Increase in OOM kills under the `kubepods/system/`
    cgroup. System Pod restarts visible via `kubectl get pods`.
  - Mitigations: Increase the system partition memory limit in
    kubelet config and restart kubelet.
  - Diagnostics: Kubelet logs will show eviction events for the
    system partition. `dmesg` will show OOM kills under the system
    partition cgroup.
  - Testing: Node e2e tests will validate eviction behavior when the
    system partition is under memory pressure.

###### What steps should be taken if SLOs are not being met to determine the problem?

1. Check if the system partition memory/CPU configuration is
   appropriately sized for the system workloads running in it.
2. Inspect `kubepods/system/memory.current` vs `memory.max` to
   determine if memory pressure is causing evictions.
3. Disable the feature gate and restart kubelet to revert to the
   default behavior.

## Implementation History

<!--
Major milestones in the lifecycle of a KEP should be tracked in this section.
Major milestones might include:
- the `Summary` and `Motivation` sections being merged, signaling SIG acceptance
- the `Proposal` section being merged, signaling agreement on a proposed design
- the date implementation started
- the first Kubernetes release where an initial version of the KEP was available
- the version of Kubernetes where the KEP graduated to general availability
- when the KEP was retired or superseded
-->

- `2026-02-04`: Initial KEP draft proposed.

## Drawbacks

<!--
Why should this KEP _not_ be implemented?
-->

## Alternatives

There are a few alternatives for system DaemonSet partitioning:

- **NRI Plugins**: NRI can manage partitions externally, but lacks
  deep integration with kubelet for metrics collection and eviction,
  leading to potential race conditions.
- **DRA (Dynamic Resource Allocation)**: DRA for Native Resources is
  moving in this direction but does not yet solve the core node
  reliability and isolation problems as effectively as a native
  partition concept.
- **DIY Solutions**: Many users have built custom solutions for
  sandboxing system daemonsets, but there is no standard winner, and
  they often struggle with memory limiting and resource accounting.
- [RedHat: Management Workload Partitioning](https://github.com/openshift/enhancements/blob/master/enhancements/workload-partitioning/management-workload-partitioning.md)

Most alternatives are covering the specific aspect of resources isolation, mostly
the CPU isolation. This KEP offers a comprehensive isolation mechanism.

### Alternative cgroup hierarchies

A key design question is how to share a memory limit between system
partition Pods and system services (kubelet, containerd). Several
cgroup hierarchy alternatives were considered:

**System Pods under `system.slice`**: Place system partition Pods
directly under `system.slice` so that a single `memory.max` covers
both system services and system Pods. However, systemd owns
`system.slice` and expects its children to be `.service` or `.scope`
units — raw cgroup directories may be cleaned up during
reconciliation. More importantly, setting `memory.max` on
`system.slice` would cap all system services (sshd, journald, udev,
etc.), not just Kubernetes-related ones.

**New top-level slice as common parent**: Create a `node-system.slice`
containing both system services and system Pods, with `memory.max`
set on the slice. This requires moving kubelet and containerd out of
`system.slice` via systemd unit overrides on every node — invasive,
fragile, and hard to manage at scale.

**Soft enforcement via combined monitoring**: Keep the existing
hierarchy unchanged and have kubelet monitor the combined memory of
`system.slice` and the system partition cgroup, evicting Pods based
on aggregate usage. This avoids hierarchy changes but provides no
hard OOM boundary — the kernel cannot enforce the combined limit
atomically, and enforcement depends on kubelet polling.

**Chosen approach: Separate cgroup under `kubepods`** (Option D): The
system partition gets its own cgroup under `kubepods/system/` with
`memory.max` set independently. The system partition's memory budget
is defined as the total system budget minus `kube-reserved` and
`system-reserved`. CPU isolation is achieved by assigning the same
`cpuset` to the system partition and system services. This builds on
the existing reserved resource model, requires no systemd hierarchy
changes, and does not fight systemd ownership. The trade-off is that
system Pods and system services cannot burst into each other's memory
— the administrator must correctly size each piece. This is
acceptable for alpha and can be revisited later.


## Infrastructure Needed (Optional)

<!--
Use this section if you need things from the project/SIG. Examples include a
new subproject, repos requested, or GitHub details. Listing these here allows a
SIG to get the process for these resources started right away.
-->
