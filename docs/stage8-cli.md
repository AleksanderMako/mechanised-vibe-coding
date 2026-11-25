# Stage 8: cmd/mvc

One command that exposes every package as a subcommand, for use from a shell
or by an agent. Human readable text by default, JSON with `-json`, stable exit
codes. The command holds no logic of its own: every subcommand parses flags,
calls one package function, and prints the result.

---

## Command layout

```
mvc <group> <action> [flags] [args]

mvc edit check    -root DIR [-json] EDITS.json
mvc edit apply    -root DIR [-no-format] [-no-build] [-json] EDITS.json
mvc ast table-case   -root DIR -file F -func TestX -case 'ELEMENT'
mvc ast add-field    -root DIR -file F -type T -field 'FIELD' [-after NAME]
mvc ast remove-field -root DIR -file F -type T -name NAME
mvc ast set-doc      -root DIR -file F -symbol SYM -doc 'TEXT'
mvc ast set-const    -root DIR -file F -name NAME -value EXPR [-doc 'TEXT']
mvc ast conformance  -root DIR -file F -type T -iface IFACE
mvc verify run    -root DIR [-pkg P]... [-lint VERSION] [-race] [-all-tests] [-skip STEP]... [-json]
mvc verify flaky  -root DIR -pkg P [-test NAME] [-runs N] [-race] [-json]
mvc verify changed -root DIR [-json]
mvc verify vendor  -root DIR
mvc symbols refs  -root DIR [-pkg P]... [-no-tests] [-strict] [-json] SYMBOL
mvc symbols impls -root DIR [-pkg P]... [-no-tests] [-strict] [-json] IFACE
mvc symbols tags  -root DIR [-pkg P]... [-strict] [-json] TYPE
mvc refactor rename      -root DIR [-pkg P]... [-no-build] [-strict] [-json] SYMBOL NEWNAME
mvc refactor move        -root DIR [-pkg P]... [-no-build] [-strict] [-json] -to IMPORTPATH -to-file FILE SYMBOL...
mvc refactor demethodize -root DIR [-pkg P]... [-no-build] [-strict] [-json] TYPE.METHOD [NEWNAME]
mvc env vars      -dir PKGDIR -type TYPE [-json]
mvc env check     -dir PKGDIR -type TYPE [-env FILE]... [-manifest FILE]... [-json]
mvc env conflicts [-env FILE]... [-manifest FILE]... [-distinct A,B,...]... [-together A,B,...]... [-requires A:B,C]... [-json]
mvc merge explain -root DIR [-json]
mvc dead list     -root DIR [-pkg P]... [-tests] [-strict] [-json]
mvc dead orphaned -root DIR [-pkg P]... [-tests] [-strict] [-json] BEFORE [AFTER]
```

## Behaviour

1. `-root` defaults to the current directory, resolved to an absolute path, for every subcommand that takes it. `-dir` has no default.
2. Flags come before positional arguments. Repeated flags (`-pkg`, `-skip`, `-env`, `-manifest`, `-distinct`, `-together`, `-requires`) accumulate.
3. Exit codes: `0` success; `1` the tool ran and reports a negative result (a `CheckError`, a failed verify step, a flaky test with failures, missing variables, rule violations, dead functions found, conflicts found); `2` usage error or any other error. The negative result is still printed in full before exiting `1`.
4. `-json` prints the package's result type encoded with `encoding/json`, indented, and nothing else on stdout. Errors always go to stderr as one line prefixed with `mvc: `.
5. Text output, one line per item, tab separated where there are columns, in the order the package returns. For `verify run` each step prints as `NAME\tOK|FAIL|SKIP\tDURATION` and the failed step's output follows verbatim. For `symbols refs` each reference prints as `FILE:LINE:COL\tKIND\tCONTEXT` (stage 10 inserts a `SYMBOL` column before `CONTEXT`, see `docs/stage10-method-sets.md` rule 37).
6. `mvc` alone, `mvc <group>` alone, `-h` or `-help` print the usage of that level to stdout and exit `0`. An unknown group or action prints the usage to stderr and exits `2`.
7. The `edit` group reads the edits file with `editset.ParseJSON`; `-` means stdin.
8. `ast` actions build one `Op` and call `astedit.Apply` with it; the written files are printed one per line.
9. `env conflicts` builds rules from the flags: `-distinct A,B` is `envcheck.Distinct("A","B")`, `-together A,B` is `Together`, `-requires A:B,C` is `Requires("A","B","C")`. Sources are read in flag order, env files first, then manifests.
10. `dead orphaned` with no `AFTER` passes `""` so the working tree is analysed; it exits `1` when `NewlyDead` is not empty.

## Text formats of negative results

- `edit check` / `edit apply` with a check error: `INDEX\tFILE\tREASON\tCOUNT` per problem; with a build error the output verbatim. `-json` prints the error struct; `edit check -json` on success prints `{"Problems": []}`.
- `verify run`: the failed step's output follows its own line verbatim. Durations are rounded to milliseconds, never shown as `0s` for a step that ran.
- `verify flaky` with failures: `runs=N\tfailures=M`, then the output. `-runs` defaults to 10.
- `verify vendor`: `synced` or `unchanged`; no `-json`.
- `env check`: `NAME\tFIELD\tREQUIRED` per missing variable, then `NAME\tSRC1,SRC2` per provided one, sorted by name; `-json` prints `{"Missing": [...], "Provided": {...}}`. Any missing variable exits `1`, required or not.
- `env conflicts`: one violation per line. Rules are built in command line order across the three flags; a malformed rule value is a flag error.
- `dead list`: `PACKAGE\tNAME\tFILE:LINE`. `dead orphaned`: a `newly dead:` header and its functions, then `revived:` and its functions, both headers always printed.
- `merge explain`: per hunk `FILE:START-END\tours=DECL|deleted\ttheirs=DECL|deleted`, then each commit indented as `  HASH[:12]\tPR\tSUBJECT` with PR as `#N` or `-`.
- Other text: `symbols impls` prints `Symbol.String()` per line, `symbols tags` prints `FIELD\tTYPE\tTAG`, `env vars` prints `NAME\tFIELD\tREQUIRED\tDEFAULT`, `verify changed` prints one pattern per line; `ast`, `refactor` and `edit apply` print the written files one per line.
- Errors with newlines are folded into one stderr line with `; `. Extra positional arguments on actions that take none are a usage error.

## Out of scope

Configuration files, colour, shell completion, interactive prompts.

Stage 9 adds `-strict` and stderr warnings to `symbols`, `refactor` and `dead`; see `docs/stage9-tolerance.md` rules 14 and 15.
