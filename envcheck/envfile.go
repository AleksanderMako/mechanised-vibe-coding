package envcheck

import (
	"fmt"
	"os"
	"strings"
)

// readEnvFile parses a dotenv file into a Source named by the path as given.
func readEnvFile(path string) (Source, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Source{}, fmt.Errorf("envcheck: read env file: %w", err)
	}
	source := Source{Name: path, Vars: map[string]string{}}
	for index, rawLine := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, found := strings.Cut(line, "=")
		if !found {
			return Source{}, fmt.Errorf("envcheck: %s line %d: missing '='", path, index+1)
		}
		// A later line overwrites an earlier one with the same key.
		source.Vars[strings.TrimSpace(key)] = parseEnvValue(strings.TrimSpace(value))
	}
	return source, nil
}

// parseEnvValue removes quotes, unescapes double quoted values and strips comments from unquoted ones.
func parseEnvValue(value string) string {
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		return value[1 : len(value)-1]
	}
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		return unescapeDoubleQuoted(value[1 : len(value)-1])
	}
	if commentStart := strings.Index(value, " #"); commentStart >= 0 {
		return strings.TrimSpace(value[:commentStart])
	}
	return value
}

// unescapeDoubleQuoted turns \n, \" and \\ into their characters and leaves other backslashes alone.
func unescapeDoubleQuoted(value string) string {
	var builder strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] == '\\' && index+1 < len(value) {
			switch value[index+1] {
			case 'n':
				builder.WriteByte('\n')
				index++
				continue
			case '"', '\\':
				builder.WriteByte(value[index+1])
				index++
				continue
			}
		}
		builder.WriteByte(value[index])
	}
	return builder.String()
}
