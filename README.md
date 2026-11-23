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
| 9     | tolerance   | Broken packages no longer block `symbols`, `refactor` and `dead`; `-strict` restores the old behaviour. |
## Usage

```sh
go install github.com/AleksanderMako/machanised-vibe-coding/cmd/mvc@latest

# check a batch of text edits without writing, edits read from stdin
mvc edit check -root . - < edits.json

# the pre-push loop on two packages, lint included
mvc verify run -pkg ./lib/foo -pkg ./lib/bar -lint v2.12.2

# every use of a symbol, as JSON for an agent
mvc symbols refs -json example.com/m/lib.Config.Name

# functions the working tree orphaned compared to main
mvc dead orphaned main
```

`mvc` alone lists the groups, `mvc <group>` its actions and
`mvc <group> <action> -h` the flags and output format. Exit code `0` is
success, `1` a negative result such as a failed check or dead code found, `2`
a usage or other error. `-json` prints the package result type and nothing else.
`symbols`, `refactor` and `dead` tolerate packages that do not compile where the
result cannot depend on them and print one `mvc: warning:` line per such package
on stderr; `-strict` turns any broken package back into an error.

## How it is built

Each stage has a contract in `docs/stageN-*.md` with the exact exported API.
One agent implements the contract, a second agent writes tests from the
contract alone without seeing the implementation. See `docs/PROTOCOL.md`.
