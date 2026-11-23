# Stage 5: envcheck

One package that checks a configuration struct against what each environment
actually provides. It reads the `arg:"env:NAME"` tags of the struct, reads
dotenv files and Kubernetes manifests, and reports what is missing where and
which value rules the code would refuse.

Pure parsing: the config struct is read from source with `go/ast`, never
executed. Works on paths given by the caller; never uses the current working
directory.

---

## Package envcheck

### Exported API

```go
package envcheck

// Var is one environment variable the config struct reads.
type Var struct {
	Name     string // environment variable name
	Field    string // struct field path such as "Database.Host"
	Required bool   // tag has "required" and no default
	Default  string // from the default:"..." tag, "" when none
}

// Vars reads the env tags of struct typeName declared in the Go package at pkgDir.
func Vars(pkgDir, typeName string) ([]Var, error)

// Source is one place that provides environment variables.
type Source struct {
	Name string            // the file path, or "path#kind/name/container" for a manifest container
	Vars map[string]string // variable to value; a reference to a secret or config map is ValueFromReference
}

// ValueFromReference marks a value that comes from a secret or config map reference.
const ValueFromReference = "<valueFrom>"

// ReadEnvFile reads a dotenv file.
func ReadEnvFile(path string) (Source, error)

// ReadManifests reads Kubernetes manifests and returns one Source per container.
func ReadManifests(paths ...string) ([]Source, error)

// Missing is a variable that no source of an environment provides.
type Missing struct {
	Var     Var
	Sources []string // the source names checked
}

// Check reports the variables that are missing from all the given sources,
// and for every variable the names of the sources that provide it.
func Check(vars []Var, sources []Source) (missing []Missing, provided map[string][]string)

// Rule is one relation between variables the code enforces at start up.
type Rule interface {
	// Describe is a one line human description.
	Describe() string
	// Violations checks the merged values of an environment.
	Violations(values map[string]string) []string
}

// Distinct requires the named variables to have different values when more than one is set.
func Distinct(names ...string) Rule

// Together requires that either all of the named variables are set or none are.
func Together(names ...string) Rule

// Requires says that when the first variable is set, all the others must be set too.
func Requires(name string, others ...string) Rule

// Conflicts merges the sources in order, later sources winning, and runs the rules.
func Conflicts(sources []Source, rules ...Rule) []string
```

### Behaviour

`Vars`:

1. `pkgDir` must hold Go files; they are parsed with `go/parser`, test files excluded. `typeName` must be a top level struct type in that package, else an error naming it.
2. Fields are walked in declaration order. A field whose type is a struct declared in the same package, a pointer to one, an inline anonymous struct, or an embedded struct, is walked recursively with the field path joined by dots; embedded fields add their type name to the path. The struct field itself produces no Var, and self-referencing structs are cut off.
3. The `arg` tag is split on commas. A part `env:NAME` gives `Name`. A bare part `env` gives the field name upper cased (go-arg behaviour). A part `required` sets `Required`. Fields without an `env` part are skipped. A `default` tag gives `Default`, and a `Default` clears `Required`.
4. Fields with the tag `arg:"-"` are skipped, including their nested structs.
5. The returned slice is in declaration order with duplicates by `Name` kept, so a variable read by two fields appears twice.

`ReadEnvFile`:

6. Lines are trimmed. Empty lines and lines starting with `#` are skipped. An optional `export ` prefix is removed. The line is split at the first `=`; a line without `=` is an error naming the line number.
7. Values are trimmed; a value wholly wrapped in matching single or double quotes loses the quotes; inside double quotes `\n`, `\"` and `\\` are unescaped. An unquoted value loses a trailing ` # comment` (space, hash); a `#` without a space before it stays. A quoted value followed by a comment is not recognised as quoted.
8. A later line with the same key wins. `Source.Name` is the path as given.

`ReadManifests`:

9. Each file may hold several YAML documents separated by `---`. Documents whose `kind` is `Deployment`, `StatefulSet`, `DaemonSet`, `Job`, `CronJob` or `Pod` are read; others are skipped. The pod spec is found at the usual path for each kind (`spec.template.spec`, `spec.jobTemplate.spec.template.spec`, `spec` for Pod).
10. Both `containers` (first) and `initContainers` yield a `Source` named `path#kind/metadata.name/containerName`. Scalar values that are not strings are rendered as strings, so `8080` becomes `"8080"`. Each `env` entry with `value` sets that value; with `valueFrom` sets `ValueFromReference`. `envFrom` entries add nothing, because their keys are unknown; the count of them is not reported.
11. A document that does not parse as YAML is an error naming the file and the document index (0-based, with the 1-based number in brackets). A document missing the pod spec path is skipped.

`Check`:

12. A variable is missing when no source has its `Name` as a key. `Missing.Sources` lists every source name in input order. `Required` does not change what is missing, it is kept on the `Var` for the caller to decide.
13. `provided` maps each variable name of `vars` present in at least one source to the names of those sources, in input order, once each. Keys no Var reads are left out. `provided` is never nil.
14. Variables with the same `Name` appear once in `missing`, with the first `Var` that reads the name. A key counts as set when present, even with an empty value.

Rules:

15. `Distinct`: when two or more of the names are set, every pair of set values must differ; one violation per equal pair names the two variables and the shared value is not printed.
16. `Together`: when at least one is set and at least one is not, one violation names the unset ones.
17. `Requires`: when `name` is set, every missing one of `others` is one violation.
18. `Conflicts` merges `sources` in order with later values overriding and runs every rule, returning the violations in rule order then variable order. A `ValueFromReference` counts as set.

### Out of scope

Executing the config, reading Helm charts or kustomize, secrets lookup, flag parsing.
