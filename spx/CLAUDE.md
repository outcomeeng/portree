---
template_version: "0.18.2"
template_source: spec-tree
---

# spx/ Directory Guide (Spec Tree)

This guide explains WHEN to invoke spec-tree skills for the **portree** product. It is a **router** — the skills contain the HOW.

---

## Structure Overview

The `spx/` tree is a durable map of the product. Nothing moves because work is "done" — specs are permanent product truth, not a backlog.

Two node types at any depth:

```text
spx/
  portree.product.md                    # Product spec (root)
  NN-{slug}.adr.md                      # Architecture decision
  NN-{slug}.pdr.md                      # Product decision
  NN-{slug}.enabler/                    # Shared infrastructure
    {slug}.md                           # Spec file
    tests/                              # Co-located tests
    PLAN.md                             # Escape hatch: deferred plan (optional)
    ISSUES.md                           # Escape hatch: known issues (optional)
    NN-{slug}.enabler/                  # Children: enablers only
  NN-{slug}.outcome/                    # Hypothesis + assertions
    {slug}.md                           # Spec file
    tests/                              # Co-located tests
    PLAN.md                             # Escape hatch: deferred plan (optional)
    ISSUES.md                           # Escape hatch: known issues (optional)
    NN-{slug}.{enabler|outcome}/        # Children: enablers and outcomes
```

---

## Key Principles

1. **Durable map**: Specs stay in place. Nothing moves because work is "done."
2. **Two node types**: Enabler (infrastructure, output is known) and outcome (hypothesis, output is a bet). Enablers can only contain enabler children. Outcomes can contain both.
3. **Co-location**: Tests live with their spec in `tests/`.
4. **Atemporal voice**: Specs state product truth. Never narrate history.
5. **Deterministic context**: The tree path defines what context an agent receives.
6. **Decision records win by hierarchy**: If a spec contradicts an ADR or PDR in its ancestry, the spec is wrong. Rewrite the spec to align with the decision record before any implementation work.
7. **Decision records updated in-place**: When a decision changes, update the ADR/PDR directly. No "superseded" workflow.
8. **Escape hatches**: PLAN.md and ISSUES.md in node directories are non-durable files left by `/handoff`. They contain deferred plans or known issues. `/contextualizing` reads them automatically. Remove when resolved.

---

## Sparse Integer Ordering

Numeric prefixes encode dependency order within each directory:

1. Lower index constrains higher index, plus that higher index's descendants.
2. Same index means independent siblings. They depend on the previous lower index, but not on each other.
3. Files and directories share one number space. The numeric prefix sorts; the type suffix identifies the artifact.
4. Insert between existing indices with the midpoint integer. Fractional indexing is the escape hatch when the integer gap is zero; avoid it when possible. Frequent fractional indices mean the directory needs restructuring.
5. Numbers are sibling-unique only. The same integer can be reused under a different parent.

Formula for N items: `i_k = 10 + floor(k * 89 / (N + 1))`

For N=7: 21, 32, 43, 54, 65, 76, 87.

```text
15-auth-strategy.adr.md              # Constrains everything at 16+
21-test-harness.enabler/             # Depends on 15; constrains 22+
32-auth.outcome/                     # Independent of billing
32-billing.outcome/                  # Independent of auth
43-integration.outcome/              # Depends on BOTH 32s
```

**ALWAYS use full path when referencing nodes** — indices are sibling-unique, not globally unique:

| Wrong                  | Correct                                     |
| ---------------------- | ------------------------------------------- |
| "32-parser.enabler"    | "21-infra.enabler/32-parser.enabler"        |
| "implement enabler-43" | "implement 21-infra.enabler/43-api.enabler" |

---

## When to Invoke Skills

### Before ANY spec-tree work → `/understanding`

**BLOCKING REQUIREMENT**

Loads the Spec Tree methodology. Emits `<SPEC_TREE_FOUNDATION>` marker. Required once per session.

### Before working on a specific node → `/contextualizing`

**BLOCKING REQUIREMENT**

Walks the tree from product root to target, reads all ancestor specs, lower-index siblings, and ADRs/PDRs.

### When creating specs or nodes → `/authoring`

Create product specs, ADRs/PDRs, enabler nodes, outcome nodes.

### When breaking down a node → `/decomposing`

Decompose when a node has too many assertions (>7) or contains independent concerns.

### When restructuring the tree → `/refactoring`

Move nodes, re-scope assertions, extract shared enablers, consolidate duplicates.

### When checking consistency → `/aligning`

Review, audit, or quality check specs. Find contradictions or gaps.

---

## Quick Reference: Skills and Agents

Skills run in the main conversation. Agents preload the skill and run autonomously as subagents, returning structured APPROVED/REJECTED verdicts. Use agents when running multiple audits in parallel; use skills when you want to discuss findings with the user.

| User Says...             | Skill                         | Agent                   |
| ------------------------ | ----------------------------- | ----------------------- |
| "Implement this outcome" | `/contextualizing`            | —                       |
| "Create an outcome"      | `/authoring`                  | —                       |
| "Add an ADR"             | `/authoring`                  | —                       |
| "This node is too big"   | `/decomposing`                | —                       |
| "Move this under that"   | `/refactoring`                | —                       |
| "Check these specs"      | `/aligning`                   | —                       |
| "Write tests for this"   | `/testing`                    | —                       |
| "Start the TDD flow"     | `/applying`                   | `applier`               |
| "Audit this PDR"         | `/auditing-product-decisions` | `pdr-auditor`           |
| "Audit test evidence"    | `/auditing-tests`             | `test-evidence-auditor` |

---

## Test Naming Convention

Test level is encoded in the filename.

### Go

| Level | Pattern                           | Example                        |
| ----- | --------------------------------- | ------------------------------ |
| 1     | `{subject}_{evidence}_l1_test.go` | `port_scenario_l1_test.go`     |
| 2     | `{subject}_{evidence}_l2_test.go` | `cli_scenario_l2_test.go`      |
| 3     | `{subject}_{evidence}_l3_test.go` | `workflow_scenario_l3_test.go` |

### L2 Binary Test Pattern

L2 tests that exercise the compiled binary follow this scaffold:

```go
var binaryPath string

func TestMain(m *testing.M) {
    // Find the project root by walking up from this test file until go.mod is found.
    _, file, _, _ := runtime.Caller(0)
    dir := filepath.Dir(file)
    for {
        if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
            break
        }
        dir = filepath.Dir(dir)
    }
    // Build the binary into a temp location.
    tmp, _ := os.MkdirTemp("", "portree-test-*")
    defer os.RemoveAll(tmp)
    binaryPath = filepath.Join(tmp, "portree")
    cmd := exec.Command("go", "build", "-o", binaryPath, filepath.Join(dir, "cmd/portree"))
    cmd.Dir = dir
    if out, err := cmd.CombinedOutput(); err != nil {
        fmt.Fprintf(os.Stderr, "build failed: %v\n%s", err, out)
        os.Exit(1)
    }
    os.Exit(m.Run())
}
```

The root-finding walk is required because `go test ./spx/...` sets the working directory to the test file's package, not the project root. Hardcoded relative paths break across worktrees with different parent directory depths.

---

## Cobra Exit-Code Contract

`SilenceErrors: true` is set on the root command. Each sub-command's `RunE` returns a non-nil error on failure. `main.go` calls `os.Exit(1)` when `cmd.Execute()` returns an error. Without this contract, commands that detect failures but call `return nil` exit 0 — indistinguishable from success to callers and CI.

The pattern:

```go
// cmd/root.go
var rootCmd = &cobra.Command{
    SilenceErrors: true,
    SilenceUsage:  true,
}

// cmd/doctor.go (or any sub-command)
RunE: func(cmd *cobra.Command, args []string) error {
    if !allChecksOK {
        return fmt.Errorf("doctor: one or more checks failed")
    }
    return nil
},

// main.go
if err := cmd.Execute(); err != nil {
    fmt.Fprintln(os.Stderr, err)
    os.Exit(1)
}
```

---

## Assertion-Test Contract

Spec assertions link to their tests inline:

```markdown
### Scenarios

- Given X, when Y, then Z ([test](tests/subject_scenario_l1_test.go))
```

Every assertion must link to at least one test file.

---

## Excluded Nodes

Nodes with specs and tests but no implementation are listed in `spx/EXCLUDE`. The `spx` CLI reads this file and skips excluded nodes when running `spx test passing`. Linting always applies — style is checked regardless of implementation existence.

`spx` never writes to project configuration files. It passes exclusion flags to each tool at invocation time.

Remove entries when implementation begins and tests should start running.

---

## Session Management

Claude Code session handoffs are stored in `.spx/sessions/` (separate from the spec tree):

```text
.spx/sessions/
├── todo/          # Available for /pickup
├── doing/         # Currently claimed
└── archive/       # Completed sessions
```

Use `/handoff` to create, `/pickup` to claim.
