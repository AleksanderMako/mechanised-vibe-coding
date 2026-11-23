package main

import (
	"context"
	"flag"
	"fmt"
	"sort"
	"strings"

	"github.com/AleksanderMako/mechanised-vibe-coding/envcheck"
)

var envGroup = group{
	name:    "env",
	summary: "config struct env tags against env files and k8s manifests (package envcheck)",
	actions: []action{
		{
			name:     "vars",
			synopsis: "-dir PKGDIR -type TYPE [-json]",
			help: `Lists the env variables struct TYPE reads, one per line as
NAME<TAB>FIELD<TAB>REQUIRED<TAB>DEFAULT with REQUIRED true or false.
With -json the []envcheck.Var.`,
			setup: setupEnvVars,
		},
		{
			name:     "check",
			synopsis: "-dir PKGDIR -type TYPE [-env FILE]... [-manifest FILE]... [-json]",
			help: `Checks that the sources provide every variable of struct TYPE. Sources are
the env files, then the manifest containers, in flag order. Prints one line per
missing variable as NAME<TAB>FIELD<TAB>REQUIRED, then one line per provided
variable, sorted by name, as NAME<TAB>SOURCE1,SOURCE2. Any missing variable
exits 1. With -json an object {"Missing": []envcheck.Missing, "Provided":
{NAME: [SOURCE...]}}, still exit 1.`,
			setup: setupEnvCheck,
		},
		{
			name:     "conflicts",
			synopsis: "[-env FILE]... [-manifest FILE]... [-distinct A,B,...]... [-together A,B,...]... [-requires A:B,C]... [-json]",
			help: `Merges the sources in flag order, env files first, then manifests, later
winning, and runs the rules in the order given. -distinct A,B is
envcheck.Distinct("A","B"), -together A,B is Together, -requires A:B,C is
Requires("A","B","C"). Prints one violation per line; any violation exits 1.
With -json a JSON array of strings, still exit 1.`,
			setup: setupEnvConflicts,
		},
	},
}

// addSourceFlags registers the repeatable -env and -manifest flags.
func addSourceFlags(flags *flag.FlagSet) (envFiles, manifests *listFlag) {
	envFiles, manifests = &listFlag{}, &listFlag{}
	flags.Var(envFiles, "env", "dotenv `FILE` (repeatable)")
	flags.Var(manifests, "manifest", "Kubernetes manifest `FILE` (repeatable)")
	return envFiles, manifests
}

// readSources reads env files first and manifests second, each in flag order (rule 9).
func readSources(envFiles, manifests []string) ([]envcheck.Source, error) {
	var sources []envcheck.Source
	for _, path := range envFiles {
		source, err := envcheck.ReadEnvFile(path)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	if len(manifests) > 0 {
		containers, err := envcheck.ReadManifests(manifests...)
		if err != nil {
			return nil, err
		}
		sources = append(sources, containers...)
	}
	return sources, nil
}

func setupEnvVars(flags *flag.FlagSet) func(context.Context, []string) error {
	dir := flags.String("dir", "", "Go package directory `PKGDIR` declaring the struct")
	typeName := flags.String("type", "", "config struct `TYPE`")
	asJSON := addJSONFlag(flags)
	return func(ctx context.Context, args []string) error {
		if len(args) != 0 {
			return usageErrorf("unexpected arguments: %v", args)
		}
		if err := requireFlags(flags, "dir", "type"); err != nil {
			return err
		}
		vars, err := envcheck.Vars(*dir, *typeName)
		if err != nil {
			return err
		}
		if *asJSON {
			return printJSON(vars)
		}
		for _, v := range vars {
			fmt.Printf("%s\t%s\t%t\t%s\n", v.Name, v.Field, v.Required, v.Default)
		}
		return nil
	}
}

// envCheckResult holds both values envcheck.Check returns, so -json prints one object.
type envCheckResult struct {
	Missing  []envcheck.Missing
	Provided map[string][]string
}

func setupEnvCheck(flags *flag.FlagSet) func(context.Context, []string) error {
	dir := flags.String("dir", "", "Go package directory `PKGDIR` declaring the struct")
	typeName := flags.String("type", "", "config struct `TYPE`")
	envFiles, manifests := addSourceFlags(flags)
	asJSON := addJSONFlag(flags)
	return func(ctx context.Context, args []string) error {
		if len(args) != 0 {
			return usageErrorf("unexpected arguments: %v", args)
		}
		if err := requireFlags(flags, "dir", "type"); err != nil {
			return err
		}
		vars, err := envcheck.Vars(*dir, *typeName)
		if err != nil {
			return err
		}
		sources, err := readSources(*envFiles, *manifests)
		if err != nil {
			return err
		}
		missing, provided := envcheck.Check(vars, sources)
		if *asJSON {
			if err := printJSON(envCheckResult{Missing: missing, Provided: provided}); err != nil {
				return err
			}
		} else {
			for _, m := range missing {
				fmt.Printf("%s\t%s\t%t\n", m.Var.Name, m.Var.Field, m.Var.Required)
			}
			// Map order is random; sorting keeps the output stable between runs.
			names := make([]string, 0, len(provided))
			for name := range provided {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				fmt.Printf("%s\t%s\n", name, strings.Join(provided[name], ","))
			}
		}
		if len(missing) > 0 {
			return errNegative
		}
		return nil
	}
}

// ruleFlag turns one -distinct, -together or -requires value into a rule on a shared list,
// so the rules keep the order they were given in across all three flags.
type ruleFlag struct {
	rules *[]envcheck.Rule
	build func(value string) (envcheck.Rule, error)
}

func (r ruleFlag) String() string { return "" }

func (r ruleFlag) Set(value string) error {
	rule, err := r.build(value)
	if err != nil {
		return err
	}
	*r.rules = append(*r.rules, rule)
	return nil
}

// splitNames splits "A,B" into names and rejects empty ones, which no rule can use.
func splitNames(value string) ([]string, error) {
	names := strings.Split(value, ",")
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("empty variable name in %q", value)
		}
	}
	return names, nil
}

func buildDistinct(value string) (envcheck.Rule, error) {
	names, err := splitNames(value)
	if err != nil {
		return nil, err
	}
	return envcheck.Distinct(names...), nil
}

func buildTogether(value string) (envcheck.Rule, error) {
	names, err := splitNames(value)
	if err != nil {
		return nil, err
	}
	return envcheck.Together(names...), nil
}

func buildRequires(value string) (envcheck.Rule, error) {
	name, others, found := strings.Cut(value, ":")
	if !found || strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("want NAME:OTHER,OTHER, got %q", value)
	}
	otherNames, err := splitNames(others)
	if err != nil {
		return nil, err
	}
	return envcheck.Requires(name, otherNames...), nil
}

func setupEnvConflicts(flags *flag.FlagSet) func(context.Context, []string) error {
	envFiles, manifests := addSourceFlags(flags)
	var rules []envcheck.Rule
	flags.Var(ruleFlag{&rules, buildDistinct}, "distinct", "variables `A,B,...` that must differ when set (repeatable)")
	flags.Var(ruleFlag{&rules, buildTogether}, "together", "variables `A,B,...` set all or none (repeatable)")
	flags.Var(ruleFlag{&rules, buildRequires}, "requires", "`A:B,C` means when A is set B and C must be too (repeatable)")
	asJSON := addJSONFlag(flags)
	return func(ctx context.Context, args []string) error {
		if len(args) != 0 {
			return usageErrorf("unexpected arguments: %v", args)
		}
		sources, err := readSources(*envFiles, *manifests)
		if err != nil {
			return err
		}
		violations := envcheck.Conflicts(sources, rules...)
		if *asJSON {
			if err := printJSON(violations); err != nil {
				return err
			}
		} else {
			printLines(violations)
		}
		if len(violations) > 0 {
			return errNegative
		}
		return nil
	}
}
