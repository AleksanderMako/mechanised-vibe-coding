package main_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/envcheck"
)

// configSource is a config package whose struct reads APP_HOST and APP_PORT.
const configSource = "package config\n\n" +
	"// Config is read from the environment.\n" +
	"type Config struct {\n" +
	"\tHost string `arg:\"env:APP_HOST,required\"`\n" +
	"\tPort string `arg:\"env:APP_PORT\"`\n" +
	"}\n"

// writeConfigPackage writes the config package into its own directory and returns it.
func writeConfigPackage(t *testing.T) string {
	t.Helper()
	dir := realTempDir(t)
	writeFile(t, dir, "config.go", configSource)
	return dir
}

// expectLineNaming checks that one stdout line names every one of names.
func expectLineNaming(t *testing.T, result runResult, names ...string) {
	t.Helper()
	for _, line := range nonEmptyLines(result.stdout) {
		namesAll := true
		for _, name := range names {
			if !strings.Contains(line, name) {
				namesAll = false
			}
		}
		if namesAll {
			return
		}
	}
	t.Errorf("no stdout line names all of %v:\n%s", names, result.describe())
}

// Rule 4: env vars -json decodes into []envcheck.Var.
func TestEnvVarsJSON(t *testing.T) {
	configDir := writeConfigPackage(t)
	result := runMVC(t, configDir, "env", "vars", "-dir", configDir, "-type", "Config", "-json")
	expectExit(t, result, 0)
	var vars []envcheck.Var
	decodeJSON(t, result, &vars)
	if len(vars) != 2 || vars[0].Name != "APP_HOST" || !vars[0].Required || vars[1].Name != "APP_PORT" {
		t.Errorf("vars = %+v, want APP_HOST (required) then APP_PORT", vars)
	}
}

// Rule 3: env check exits 0 when every variable is provided.
func TestEnvCheckNothingMissing(t *testing.T) {
	configDir := writeConfigPackage(t)
	envDir := realTempDir(t)
	envFile := writeFile(t, envDir, "full.env", "APP_HOST=localhost\nAPP_PORT=8080\n")
	result := runMVC(t, envDir, "env", "check", "-dir", configDir, "-type", "Config", "-env", envFile)
	expectExit(t, result, 0)
}

// Rule 3: env check exits 1 and names the missing variable.
func TestEnvCheckMissingVariable(t *testing.T) {
	configDir := writeConfigPackage(t)
	envDir := realTempDir(t)
	envFile := writeFile(t, envDir, "partial.env", "APP_PORT=8080\n")

	result := runMVC(t, envDir, "env", "check", "-dir", configDir, "-type", "Config", "-env", envFile)
	expectExit(t, result, 1)
	if !strings.Contains(result.stdout, "APP_HOST") {
		t.Errorf("missing APP_HOST not printed:\n%s", result.describe())
	}

	jsonResult := runMVC(t, envDir, "env", "check", "-dir", configDir, "-type", "Config", "-env", envFile, "-json")
	expectExit(t, jsonResult, 1)
	if !json.Valid([]byte(jsonResult.stdout)) || !strings.Contains(jsonResult.stdout, "APP_HOST") {
		t.Errorf("want JSON naming APP_HOST:\n%s", jsonResult.describe())
	}
}

// Rule 2: repeated -env flags accumulate, so two partial files together provide everything.
func TestEnvCheckRepeatedEnv(t *testing.T) {
	configDir := writeConfigPackage(t)
	envDir := realTempDir(t)
	hostFile := writeFile(t, envDir, "host.env", "APP_HOST=localhost\n")
	portFile := writeFile(t, envDir, "port.env", "APP_PORT=8080\n")
	result := runMVC(t, envDir, "env", "check", "-dir", configDir, "-type", "Config", "-env", hostFile, "-env", portFile)
	expectExit(t, result, 0)
}

// Rule 9: each rule flag gives exit 1 with a violation line when broken and exit 0 when kept.
func TestEnvConflictsRules(t *testing.T) {
	envDir := realTempDir(t)
	envFile := writeFile(t, envDir, "app.env", "PRIMARY_URL=same\nREPLICA_URL=same\nOTHER_URL=different\nTLS_CERT=cert\nTLS_KEY=key\nAUTH_ON=yes\nAUTH_USER=admin\n")
	cases := []struct {
		name      string
		ruleFlags []string
		wantExit  int
		wantNamed []string // names one violation line must contain
	}{
		{name: "distinct broken", ruleFlags: []string{"-distinct", "PRIMARY_URL,REPLICA_URL"}, wantExit: 1, wantNamed: []string{"PRIMARY_URL", "REPLICA_URL"}},
		{name: "distinct kept", ruleFlags: []string{"-distinct", "PRIMARY_URL,OTHER_URL"}, wantExit: 0},
		{name: "together broken", ruleFlags: []string{"-together", "TLS_CERT,TLS_CA"}, wantExit: 1, wantNamed: []string{"TLS_CERT", "TLS_CA"}},
		{name: "together kept", ruleFlags: []string{"-together", "TLS_CERT,TLS_KEY"}, wantExit: 0},
		{name: "requires broken", ruleFlags: []string{"-requires", "AUTH_ON:AUTH_USER,AUTH_PASSWORD"}, wantExit: 1, wantNamed: []string{"AUTH_ON", "AUTH_PASSWORD"}},
		{name: "requires kept", ruleFlags: []string{"-requires", "AUTH_ON:AUTH_USER"}, wantExit: 0},
		{name: "repeated rule flags accumulate", ruleFlags: []string{"-distinct", "PRIMARY_URL,OTHER_URL", "-requires", "AUTH_ON:AUTH_PASSWORD"}, wantExit: 1, wantNamed: []string{"AUTH_ON", "AUTH_PASSWORD"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			args := append([]string{"env", "conflicts", "-env", envFile}, testCase.ruleFlags...)
			result := runMVC(t, envDir, args...)
			expectExit(t, result, testCase.wantExit)
			if testCase.wantExit == 0 && strings.TrimSpace(result.stdout) != "" {
				t.Errorf("no violation expected:\n%s", result.describe())
			}
			if len(testCase.wantNamed) > 0 {
				expectLineNaming(t, result, testCase.wantNamed...)
			}
		})
	}
}

// Rule 4: env conflicts -json prints valid JSON naming the violation.
func TestEnvConflictsJSON(t *testing.T) {
	envDir := realTempDir(t)
	envFile := writeFile(t, envDir, "app.env", "PRIMARY_URL=same\nREPLICA_URL=same\n")
	result := runMVC(t, envDir, "env", "conflicts", "-env", envFile, "-distinct", "PRIMARY_URL,REPLICA_URL", "-json")
	expectExit(t, result, 1)
	var violations []string
	decodeJSON(t, result, &violations)
	if len(violations) != 1 || !strings.Contains(violations[0], "PRIMARY_URL") {
		t.Errorf("violations = %v, want one naming PRIMARY_URL", violations)
	}
}

// Rule 9: env files are read in flag order, later ones winning.
func TestEnvConflictsEnvFileOrder(t *testing.T) {
	envDir := realTempDir(t)
	firstFile := writeFile(t, envDir, "first.env", "PRIMARY_URL=a\nREPLICA_URL=b\n")
	overrideFile := writeFile(t, envDir, "override.env", "REPLICA_URL=a\n")

	overrideLast := runMVC(t, envDir, "env", "conflicts", "-env", firstFile, "-env", overrideFile, "-distinct", "PRIMARY_URL,REPLICA_URL")
	expectExit(t, overrideLast, 1)

	overrideFirst := runMVC(t, envDir, "env", "conflicts", "-env", overrideFile, "-env", firstFile, "-distinct", "PRIMARY_URL,REPLICA_URL")
	expectExit(t, overrideFirst, 0)
}

// Rule 9: manifests are read after env files even when -manifest comes first on the command line.
func TestEnvConflictsManifestsAfterEnvFiles(t *testing.T) {
	envDir := realTempDir(t)
	envFile := writeFile(t, envDir, "app.env", "PRIMARY_URL=a\nREPLICA_URL=b\n")
	manifest := writeFile(t, envDir, "deploy.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
spec:
  template:
    spec:
      containers:
        - name: server
          env:
            - name: REPLICA_URL
              value: a
`)
	result := runMVC(t, envDir, "env", "conflicts", "-manifest", manifest, "-env", envFile, "-distinct", "PRIMARY_URL,REPLICA_URL")
	expectExit(t, result, 1)
	expectLineNaming(t, result, "PRIMARY_URL", "REPLICA_URL")
}
