package envcheck

import (
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// podSpecPaths gives, per workload kind, the keys leading from the document root to the pod spec.
var podSpecPaths = map[string][]string{
	"Deployment":  {"spec", "template", "spec"},
	"StatefulSet": {"spec", "template", "spec"},
	"DaemonSet":   {"spec", "template", "spec"},
	"Job":         {"spec", "template", "spec"},
	"CronJob":     {"spec", "jobTemplate", "spec", "template", "spec"},
	"Pod":         {"spec"},
}

// readManifestFile returns one Source per container of every workload document in the file.
func readManifestFile(path string) ([]Source, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("envcheck: open manifest: %w", err)
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	var sources []Source
	for documentIndex := 0; ; documentIndex++ {
		var document any
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			return sources, nil
		}
		if err != nil {
			return nil, fmt.Errorf("envcheck: %s: document index %d (document %d): %w", path, documentIndex, documentIndex+1, err)
		}
		sources = append(sources, documentSources(path, document)...)
	}
}

// documentSources reads the containers of one decoded YAML document; non workloads yield nothing.
func documentSources(path string, document any) []Source {
	root := asMap(document)
	kind, _ := root["kind"].(string)
	specPath, isWorkload := podSpecPaths[kind]
	if !isWorkload {
		return nil
	}
	podSpec := root
	for _, key := range specPath {
		podSpec = asMap(podSpec[key])
		if podSpec == nil {
			return nil
		}
	}
	name, _ := asMap(root["metadata"])["name"].(string)
	var sources []Source
	for _, listKey := range []string{"containers", "initContainers"} {
		containers, _ := podSpec[listKey].([]any)
		for _, rawContainer := range containers {
			container := asMap(rawContainer)
			containerName, _ := container["name"].(string)
			sources = append(sources, Source{
				Name: fmt.Sprintf("%s#%s/%s/%s", path, kind, name, containerName),
				Vars: containerEnv(container),
			})
		}
	}
	return sources
}

// containerEnv maps each env entry to its value, or to ValueFromReference for valueFrom; envFrom keys are unknown and skipped.
func containerEnv(container map[string]any) map[string]string {
	vars := map[string]string{}
	entries, _ := container["env"].([]any)
	for _, rawEntry := range entries {
		entry := asMap(rawEntry)
		name, _ := entry["name"].(string)
		if name == "" {
			continue
		}
		if _, hasReference := entry["valueFrom"]; hasReference {
			vars[name] = ValueFromReference
			continue
		}
		value := entry["value"]
		if value == nil {
			// An entry with only a name sets the variable to the empty string.
			vars[name] = ""
			continue
		}
		vars[name] = fmt.Sprint(value)
	}
	return vars
}

// asMap converts a decoded YAML mapping to map[string]any, returning nil for anything else.
func asMap(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case map[any]any:
		converted := make(map[string]any, len(typed))
		for key, inner := range typed {
			converted[fmt.Sprint(key)] = inner
		}
		return converted
	}
	return nil
}
