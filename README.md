# machanised-vibe-coding

Deterministic Go tooling for the clerical parts of agent assisted development.
Every tool here replaces a step where a language model was slow, or wrong, or
both, during a real refactoring session: text edits that land half way, AST
edits done with fragile byte anchors, the gofmt-build-vet-test-lint loop,
"who calls this", env vars missing for an environment, merge conflicts whose
intent has to be dug out of git, and code a refactor orphaned.

The tools are plain Go packages with a thin `mvc` command on top. They know
about Go modules and git, nothing about any one project.

## Packages

| Stage | Package     | What it does                                                                 |
|-------|-------------|------------------------------------------------------------------------------|
| 1     | `editset`   | Transactional text edits across files: check every anchor, write all or none, goimports, build. |
| 1     | `astedit`   | AST anchored edits that keep comments: table test case, struct field, doc comment, const, conformance line. |
| 2     | `verify`    | The fixed pre-push loop: gofmt, vendor drift, build, vet, test, lint; flaky test hunting. |
| 3     | `symbols`   | Type aware impact analysis: references, implementations, struct tags.        |
| 4     | `refactor`  | Rename, move to another package, demethodize, each rewriting every reference. |
| 5     | `envcheck`  | Config struct env tags against env files and k8s manifests per environment.  |
| 6     | `mergeinfo` | Classify merge conflicts: enclosing declarations, deletions, their commits.  |
| 7     | `deadcode`  | What a change orphaned: whole module reachability before and after.          |
| 8     | `cmd/mvc`   | One command, one subcommand per package.                                     |

## How it is built

Each stage has a contract in `docs/stageN-*.md` with the exact exported API.
One agent implements the contract, a second agent writes tests from the
contract alone without seeing the implementation. See `docs/PROTOCOL.md`.
