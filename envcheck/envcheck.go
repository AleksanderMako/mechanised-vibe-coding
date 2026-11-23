// Package envcheck compares the env tags of a config struct with what dotenv
// files and Kubernetes manifests provide, and checks value rules the code enforces.
package envcheck

// Var is one environment variable the config struct reads.
type Var struct {
	Name     string // environment variable name
	Field    string // struct field path such as "Database.Host"
	Required bool   // tag has "required" and no default
	Default  string // from the default:"..." tag
}

// Vars reads the env tags of struct typeName declared in the Go package at pkgDir.
func Vars(pkgDir, typeName string) ([]Var, error) { panic("not implemented") }

// Source is one place that provides environment variables.
type Source struct {
	Name string            // file path, or "path#kind/name/container" for a manifest container
	Vars map[string]string // variable to value
}

// ValueFromReference marks a value that comes from a secret or config map reference.
const ValueFromReference = "<valueFrom>"

// ReadEnvFile reads a dotenv file.
func ReadEnvFile(path string) (Source, error) { panic("not implemented") }

// ReadManifests reads Kubernetes manifests and returns one Source per container.
func ReadManifests(paths ...string) ([]Source, error) { panic("not implemented") }

// Missing is a variable that no source provides.
type Missing struct {
	Var     Var
	Sources []string // the source names checked
}

// Check reports the variables missing from all sources and who provides the rest.
func Check(vars []Var, sources []Source) (missing []Missing, provided map[string][]string) {
	panic("not implemented")
}

// Rule is one relation between variables the code enforces at start up.
type Rule interface {
	// Describe is a one line human description.
	Describe() string
	// Violations checks the merged values of an environment.
	Violations(values map[string]string) []string
}

// Distinct requires the named variables to differ when more than one is set.
func Distinct(names ...string) Rule { panic("not implemented") }

// Together requires all of the named variables to be set, or none.
func Together(names ...string) Rule { panic("not implemented") }

// Requires says that when name is set, all of others must be set too.
func Requires(name string, others ...string) Rule { panic("not implemented") }

// Conflicts merges the sources in order, later winning, and runs the rules.
func Conflicts(sources []Source, rules ...Rule) []string { panic("not implemented") }
