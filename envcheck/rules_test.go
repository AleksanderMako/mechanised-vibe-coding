package envcheck_test

import (
	"strings"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/envcheck"
)

// sharedSecret is a value that must never appear in a Distinct violation.
const sharedSecret = "s3cr3t-shared-value"

// Every rule describes itself on one non-empty line.
func TestRulesDescribeOnOneLine(t *testing.T) {
	rules := map[string]envcheck.Rule{
		"Distinct": envcheck.Distinct("A", "B"),
		"Together": envcheck.Together("A", "B"),
		"Requires": envcheck.Requires("A", "B"),
	}
	for kind, rule := range rules {
		description := rule.Describe()
		if strings.TrimSpace(description) == "" {
			t.Errorf("%s: empty description", kind)
		}
		if strings.Contains(description, "\n") {
			t.Errorf("%s: description spans lines: %q", kind, description)
		}
	}
}

// Rule 15: two equal set values give one violation naming both, without the value.
func TestDistinctNamesBothVariablesAndHidesValue(t *testing.T) {
	rule := envcheck.Distinct("READ_DSN", "WRITE_DSN", "CACHE_DSN")
	violations := rule.Violations(map[string]string{
		"READ_DSN":  sharedSecret,
		"WRITE_DSN": sharedSecret,
		"CACHE_DSN": "other-value",
	})
	if len(violations) != 1 {
		t.Fatalf("want 1 violation, got %q", violations)
	}
	assertContainsAll(t, violations[0], "READ_DSN", "WRITE_DSN")
	if strings.Contains(violations[0], sharedSecret) {
		t.Errorf("violation must not print the shared value: %q", violations[0])
	}
}

// Rule 15: every pair of equal set values is its own violation.
func TestDistinctReportsEveryEqualPair(t *testing.T) {
	rule := envcheck.Distinct("A_URL", "B_URL", "C_URL")
	violations := rule.Violations(map[string]string{"A_URL": sharedSecret, "B_URL": sharedSecret, "C_URL": sharedSecret})
	if len(violations) != 3 {
		t.Fatalf("three equal values form three pairs: want 3 violations, got %q", violations)
	}
	for _, violation := range violations {
		if strings.Contains(violation, sharedSecret) {
			t.Errorf("violation must not print the shared value: %q", violation)
		}
	}
}

// Rule 15: different values, or fewer than two set, are fine.
func TestDistinctAcceptsDifferentOrSingleValues(t *testing.T) {
	rule := envcheck.Distinct("A_URL", "B_URL")
	if violations := rule.Violations(map[string]string{"A_URL": "one", "B_URL": "two"}); len(violations) != 0 {
		t.Errorf("different values: want no violation, got %q", violations)
	}
	if violations := rule.Violations(map[string]string{"A_URL": sharedSecret}); len(violations) != 0 {
		t.Errorf("only one set: want no violation, got %q", violations)
	}
	if violations := rule.Violations(map[string]string{}); len(violations) != 0 {
		t.Errorf("none set: want no violation, got %q", violations)
	}
}

// Rule 16: some set and some not gives one violation naming the unset ones.
func TestTogetherNamesUnsetVariables(t *testing.T) {
	rule := envcheck.Together("TLS_CERT", "TLS_KEY", "TLS_CA")
	violations := rule.Violations(map[string]string{"TLS_CERT": "cert.pem"})
	if len(violations) != 1 {
		t.Fatalf("want 1 violation, got %q", violations)
	}
	assertContainsAll(t, violations[0], "TLS_KEY", "TLS_CA")
}

// Rule 16: all set or none set is fine.
func TestTogetherAcceptsAllOrNone(t *testing.T) {
	rule := envcheck.Together("TLS_CERT", "TLS_KEY")
	if violations := rule.Violations(map[string]string{"TLS_CERT": "c", "TLS_KEY": "k"}); len(violations) != 0 {
		t.Errorf("all set: want no violation, got %q", violations)
	}
	if violations := rule.Violations(map[string]string{"UNRELATED": "x"}); len(violations) != 0 {
		t.Errorf("none set: want no violation, got %q", violations)
	}
}

// Rule 17: each missing dependency of a set variable is one violation.
func TestRequiresOneViolationPerMissingOther(t *testing.T) {
	rule := envcheck.Requires("SMTP_HOST", "SMTP_PASSWORD", "SMTP_USER")
	violations := rule.Violations(map[string]string{"SMTP_HOST": "mail"})
	if len(violations) != 2 {
		t.Fatalf("want 2 violations, got %q", violations)
	}
	assertContainsAll(t, strings.Join(violations, "\n"), "SMTP_PASSWORD", "SMTP_USER")
	if strings.Contains(violations[0], "SMTP_PASSWORD") == strings.Contains(violations[0], "SMTP_USER") {
		t.Errorf("each violation should name one missing variable, got %q", violations)
	}

	violations = rule.Violations(map[string]string{"SMTP_HOST": "mail", "SMTP_USER": "me"})
	if len(violations) != 1 {
		t.Fatalf("want 1 violation, got %q", violations)
	}
	assertContainsAll(t, violations[0], "SMTP_PASSWORD")
}

// Rule 17: when the first variable is unset nothing is required.
func TestRequiresIgnoresUnsetTrigger(t *testing.T) {
	rule := envcheck.Requires("SMTP_HOST", "SMTP_USER")
	if violations := rule.Violations(map[string]string{}); len(violations) != 0 {
		t.Errorf("trigger unset: want no violation, got %q", violations)
	}
	if violations := rule.Violations(map[string]string{"SMTP_HOST": "mail", "SMTP_USER": "me"}); len(violations) != 0 {
		t.Errorf("all present: want no violation, got %q", violations)
	}
}

// Rule 18: later sources override earlier ones before the rules run.
func TestConflictsLaterSourceWins(t *testing.T) {
	sources := []envcheck.Source{
		{Name: "base.env", Vars: map[string]string{"READ_DSN": sharedSecret, "WRITE_DSN": sharedSecret}},
		{Name: "override.env", Vars: map[string]string{"WRITE_DSN": "different"}},
	}
	if violations := envcheck.Conflicts(sources, envcheck.Distinct("READ_DSN", "WRITE_DSN")); len(violations) != 0 {
		t.Errorf("override.env changes WRITE_DSN: want no violation, got %q", violations)
	}

	sources = []envcheck.Source{
		{Name: "base.env", Vars: map[string]string{"READ_DSN": sharedSecret, "WRITE_DSN": "different"}},
		{Name: "override.env", Vars: map[string]string{"WRITE_DSN": sharedSecret}},
	}
	violations := envcheck.Conflicts(sources, envcheck.Distinct("READ_DSN", "WRITE_DSN"))
	if len(violations) != 1 {
		t.Fatalf("override.env makes values equal: want 1 violation, got %q", violations)
	}
	assertContainsAll(t, violations[0], "READ_DSN", "WRITE_DSN")
	if strings.Contains(violations[0], sharedSecret) {
		t.Errorf("violation must not print the shared value: %q", violations[0])
	}
}

// Rule 18: violations come in rule order, then variable order within a rule.
func TestConflictsOrdersByRuleThenVariable(t *testing.T) {
	sources := []envcheck.Source{
		{Name: "a.env", Vars: map[string]string{"SMTP_HOST": "mail"}},
		{Name: "b.env", Vars: map[string]string{"TLS_CERT": "cert.pem"}},
	}
	violations := envcheck.Conflicts(sources,
		envcheck.Requires("SMTP_HOST", "SMTP_A_USER", "SMTP_B_PASSWORD"),
		envcheck.Together("TLS_CERT", "TLS_KEY"),
	)
	if len(violations) != 3 {
		t.Fatalf("want 3 violations, got %q", violations)
	}
	assertContainsAll(t, violations[0], "SMTP_A_USER")
	assertContainsAll(t, violations[1], "SMTP_B_PASSWORD")
	assertContainsAll(t, violations[2], "TLS_KEY")
}

// Rule 18: a value from a secret or config map reference counts as set.
func TestConflictsValueFromReferenceCountsAsSet(t *testing.T) {
	sources := []envcheck.Source{
		{Name: "app.yaml#Deployment/web/server", Vars: map[string]string{
			"SMTP_HOST": envcheck.ValueFromReference,
			"TLS_CERT":  envcheck.ValueFromReference,
			"TLS_KEY":   envcheck.ValueFromReference,
		}},
	}
	violations := envcheck.Conflicts(sources,
		envcheck.Requires("SMTP_HOST", "SMTP_USER"),
		envcheck.Together("TLS_CERT", "TLS_KEY"),
	)
	if len(violations) != 1 {
		t.Fatalf("referenced SMTP_HOST is set and needs SMTP_USER; TLS pair is complete: want 1 violation, got %q", violations)
	}
	assertContainsAll(t, violations[0], "SMTP_USER")
}

// Rule 18: no rules, or rules that hold, give no violations.
func TestConflictsWithoutViolations(t *testing.T) {
	sources := []envcheck.Source{{Name: "a.env", Vars: map[string]string{"A": "1", "B": "2"}}}
	if violations := envcheck.Conflicts(sources); len(violations) != 0 {
		t.Errorf("no rules: want no violation, got %q", violations)
	}
	if violations := envcheck.Conflicts(sources, envcheck.Distinct("A", "B"), envcheck.Together("A", "B")); len(violations) != 0 {
		t.Errorf("rules hold: want no violation, got %q", violations)
	}
}
