# Development protocol

Every stage is built by two agents working from the same contract document.

## The contract

`docs/stageN-<name>.md` holds, for each package of the stage:

- the purpose in a few sentences,
- the exact exported Go API, already present as compiling stubs in the package,
- the behaviour of each function including every error case,
- what is explicitly out of scope.

The contract is the only shared truth. Neither agent changes an exported
signature on its own. If a signature cannot work, the agent reports why and
stops on that point instead of inventing a replacement.

## The implementer

Works in the main checkout. Replaces the stub bodies, adds unexported helpers
and files as needed, keeps the exported API as the contract states it. Writes
no tests except ad hoc ones it deletes again. Finishes with `gofmt -l` clean,
`go vet ./...` clean and `go build ./...` passing.

## The tester

Works in a separate git worktree created from the commit that holds only the
stubs, so the implementation is not visible. Writes external tests
(`package <name>_test`) against the contract, with fixtures under
`<pkg>/testdata`. Tests must compile against the stubs. Tests assert the
behaviour the contract promises, including error cases, and nothing else.

## Merging

The orchestrator copies the tests into the main checkout and runs them.
Each failure is routed: a contract ambiguity is settled by editing the
contract, an implementation bug goes back to the implementer, a test that
asserts something the contract does not promise goes back to the tester.
The tester never receives implementation code, only the failing output.

## Conventions

- Go 1.26, standard library first; `golang.org/x/tools`, `github.com/dave/dst`
  and `gopkg.in/yaml.v3` are the only allowed dependencies.
- Names a novice reads without context. One line comments that say why.
- Tools take a `Root` directory and never depend on the current working directory.
- Tools shell out to the `go` tool and `git` only; never to editors or gopls.
- No commit attribution lines.
