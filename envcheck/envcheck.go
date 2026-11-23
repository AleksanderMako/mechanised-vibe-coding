// Package envcheck compares the env tags of a config struct with what dotenv
// files and Kubernetes manifests provide, and checks value rules the code enforces.
package envcheck

import (
	"fmt"
	"slices"
	"strings"
)

// Var is one environment variable the config struct reads.
type Var struct {
	Name     string // environment variable name
	Field    string // struct field path such as "Database.Host"
	Required bool   // tag has "required" and no default
	Default  string // from the default:"..." tag
}

// Vars reads the env tags of struct typeName declared in the Go package at pkgDir.
func Vars(pkgDir, typeName string) ([]Var, error) {
	structTypes, err := parsePackageStructs(pkgDir)
	if err != nil {
		return nil, err
	}
	root, found := structTypes[typeName]
	if !found {
		return nil, fmt.Errorf("envcheck: struct type %q not found in %s", typeName, pkgDir)
	}
	walker := fieldWalker{structTypes: structTypes, visiting: map[string]bool{typeName: true}}
	walker.walk(root, "")
	return walker.vars, nil
}

// Source is one place that provides environment variables.
type Source struct {
	Name string            // file path, or "path#kind/name/container" for a manifest container
	Vars map[string]string // variable to value
}

// ValueFromReference marks a value that comes from a secret or config map reference.
const ValueFromReference = "<valueFrom>"

// ReadEnvFile reads a dotenv file.
func ReadEnvFile(path string) (Source, error) { return readEnvFile(path) }

// ReadManifests reads Kubernetes manifests and returns one Source per container.
func ReadManifests(paths ...string) ([]Source, error) {
	var sources []Source
	for _, path := range paths {
		fileSources, err := readManifestFile(path)
		if err != nil {
			return nil, err
		}
		sources = append(sources, fileSources...)
	}
	return sources, nil
}

// Missing is a variable that no source provides.
type Missing struct {
	Var     Var
	Sources []string // the source names checked
}

// Check reports the variables missing from all sources and who provides the rest.
func Check(vars []Var, sources []Source) (missing []Missing, provided map[string][]string) {
	provided = map[string][]string{}
	sourceNames := make([]string, 0, len(sources))
	for _, source := range sources {
		sourceNames = append(sourceNames, source.Name)
	}
	// A name read by several fields is reported once, at its first field.
	seen := map[string]bool{}
	for _, variable := range vars {
		if seen[variable.Name] {
			continue
		}
		seen[variable.Name] = true
		var providers []string
		for _, source := range sources {
			if _, ok := source.Vars[variable.Name]; ok {
				providers = append(providers, source.Name)
			}
		}
		if len(providers) == 0 {
			missing = append(missing, Missing{Var: variable, Sources: slices.Clone(sourceNames)})
			continue
		}
		provided[variable.Name] = providers
	}
	return missing, provided
}

// Rule is one relation between variables the code enforces at start up.
type Rule interface {
	// Describe is a one line human description.
	Describe() string
	// Violations checks the merged values of an environment.
	Violations(values map[string]string) []string
}

// Distinct requires the named variables to differ when more than one is set.
func Distinct(names ...string) Rule { return distinctRule{names: slices.Clone(names)} }

// Together requires all of the named variables to be set, or none.
func Together(names ...string) Rule { return togetherRule{names: slices.Clone(names)} }

// Requires says that when name is set, all of others must be set too.
func Requires(name string, others ...string) Rule {
	return requiresRule{name: name, others: slices.Clone(others)}
}

// Conflicts merges the sources in order, later winning, and runs the rules.
func Conflicts(sources []Source, rules ...Rule) []string {
	merged := map[string]string{}
	for _, source := range sources {
		for name, value := range source.Vars {
			merged[name] = value
		}
	}
	var violations []string
	for _, rule := range rules {
		violations = append(violations, rule.Violations(merged)...)
	}
	return violations
}

// distinctRule is the Rule returned by Distinct.
type distinctRule struct{ names []string }

func (rule distinctRule) Describe() string {
	return strings.Join(rule.names, ", ") + " must have different values"
}

func (rule distinctRule) Violations(values map[string]string) []string {
	setNames := setOnly(rule.names, values)
	var violations []string
	for first := 0; first < len(setNames); first++ {
		for second := first + 1; second < len(setNames); second++ {
			if values[setNames[first]] == values[setNames[second]] {
				// The value itself may be a secret, so only the names are printed.
				violations = append(violations, fmt.Sprintf("%s and %s have the same value", setNames[first], setNames[second]))
			}
		}
	}
	return violations
}

// togetherRule is the Rule returned by Together.
type togetherRule struct{ names []string }

func (rule togetherRule) Describe() string {
	return strings.Join(rule.names, ", ") + " must be set together or not at all"
}

func (rule togetherRule) Violations(values map[string]string) []string {
	setNames := setOnly(rule.names, values)
	var unsetNames []string
	for _, name := range rule.names {
		if _, ok := values[name]; !ok {
			unsetNames = append(unsetNames, name)
		}
	}
	if len(setNames) == 0 || len(unsetNames) == 0 {
		return nil
	}
	return []string{fmt.Sprintf("%s set but %s not set", strings.Join(setNames, ", "), strings.Join(unsetNames, ", "))}
}

// requiresRule is the Rule returned by Requires.
type requiresRule struct {
	name   string
	others []string
}

func (rule requiresRule) Describe() string {
	return fmt.Sprintf("when %s is set, %s must be set", rule.name, strings.Join(rule.others, ", "))
}

func (rule requiresRule) Violations(values map[string]string) []string {
	if _, ok := values[rule.name]; !ok {
		return nil
	}
	var violations []string
	for _, other := range rule.others {
		if _, ok := values[other]; !ok {
			violations = append(violations, fmt.Sprintf("%s is set but %s is not set", rule.name, other))
		}
	}
	return violations
}

// setOnly keeps the names present in values, in the given order.
func setOnly(names []string, values map[string]string) []string {
	var setNames []string
	for _, name := range names {
		if _, ok := values[name]; ok {
			setNames = append(setNames, name)
		}
	}
	return setNames
}
