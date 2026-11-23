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
mvc symbols refs  -root DIR [-pkg P]... [-no-tests] [-json] SYMBOL
mvc symbols impls -root DIR [-pkg P]... [-no-tests] [-json] IFACE
mvc symbols tags  -root DIR [-pkg P]... [-json] TYPE
mvc refactor rename      -root DIR [-pkg P]... [-no-build] [-json] SYMBOL NEWNAME
mvc refactor move        -root DIR [-pkg P]... [-no-build] [-json] -to IMPORTPATH -to-file FILE SYMBOL...
mvc refactor demethodize -root DIR [-pkg P]... [-no-build] [-json] TYPE.METHOD [NEWNAME]
mvc env vars      -dir PKGDIR -type TYPE [-json]
mvc env check     -dir PKGDIR -type TYPE [-env FILE]... [-manifest FILE]... [-json]
mvc env conflicts [-env FILE]... [-manifest FILE]... [-distinct A,B,...]... [-together A,B,...]... [-requires A:B,C]... [-json]
mvc merge explain -root DIR [-json]
mvc dead list     -root DIR [-pkg P]... [-tests] [-json]
mvc dead orphaned -root DIR [-pkg P]... [-tests] [-json] BEFORE [AFTER]
```

## Behaviour

1. `-root` defaults to the current directory, resolved to an absolute path, for every subcommand that takes it. `-dir` has no default.
2. Flags come before positional arguments. Repeated flags (`-pkg`, `-skip`, `-env`, `-manifest`, `-distinct`, `-together`, `-requires`) accumulate.
3. Exit codes: `0` success; `1` the tool ran and reports a negative result (a `CheckError`, a failed verify step, a flaky test with failures, missing variables, rule violations, dead functions found, conflicts found); `2` usage error or any other error. The negative result is still printed in full before exiting `1`.
4. `-json` prints the package's result type encoded with `encoding/json`, indented, and nothing else on stdout. Errors always go to stderr as one line prefixed with `mvc: `.
5. Text output, one line per item, tab separated where there are columns, in the order the package returns. For `verify run` each step prints as `NAME\tOK|FAIL|SKIP\tDURATION` and the failed step's output follows verbatim. For `symbols refs` each reference prints as `FILE:LINE:COL\tKIND\tCONTEXT`.
6. `mvc` alone, `mvc <group>` alone, `-h` or `-help` print the usage of that level to stdout and exit `0`. An unknown group or action prints the usage to stderr and exits `2`.
7. The `edit` group reads the edits file with `editset.ParseJSON`; `-` means stdin.
8. `ast` actions build one `Op` and call `astedit.Apply` with it; the written files are printed one per line.
9. `env conflicts` builds rules from the flags: `-distinct A,B` is `envcheck.Distinct("A","B")`, `-together A,B` is `Together`, `-requires A:B,C` is `Requires("A","B","C")`. Sources are read in flag order, env files first, then manifests.
10. `dead orphaned` with no `AFTER` passes `""` so the working tree is analysed.

## Out of scope

Configuration files, colour, shell completion, interactive prompts.
