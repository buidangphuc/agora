package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
)

var envKeyRe = regexp.MustCompile(`^([A-Z][A-Z0-9_]*)=`)

// DeclaredEnvKeys returns the sorted env variable names declared by the struct
// v (or pointer to struct), including nested structs.
func DeclaredEnvKeys(v any) []string {
	set := map[string]struct{}{}
	for _, f := range envFields(reflect.TypeOf(v)) {
		set[f.key] = struct{}{}
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

type envField struct {
	key  string
	name string // Go field name
	typ  reflect.Type
	tag  reflect.StructTag
}

// envFields lists the fields carrying an env tag, descending into nested
// structs and pointers to structs.
func envFields(t reflect.Type) []envField {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}
	var out []envField
	for f := range t.Fields() {
		if !f.IsExported() {
			continue
		}
		if tag, ok := f.Tag.Lookup("env"); ok {
			if key, _, _ := strings.Cut(tag, ","); key != "" {
				out = append(out, envField{key: key, name: f.Name, typ: f.Type, tag: f.Tag})
			}
			continue
		}
		out = append(out, envFields(f.Type)...)
	}
	return out
}

// CheckEnvExample compares the keys assigned in the .env.example at path with
// the env keys declared by v, in both directions.
func CheckEnvExample(path string, v any) error {
	documented, err := exampleKeys(path)
	if err != nil {
		return err
	}
	declared := DeclaredEnvKeys(v)

	var missing, stale []string
	for _, k := range declared {
		if _, ok := documented[k]; !ok {
			missing = append(missing, k)
		}
	}
	for k := range documented {
		if !slices.Contains(declared, k) {
			stale = append(stale, k)
		}
	}
	if len(missing) == 0 && len(stale) == 0 {
		return nil
	}
	slices.Sort(stale)

	var b strings.Builder
	fmt.Fprintf(&b, "%s is out of sync with the settings struct:", path)
	if len(missing) > 0 {
		b.WriteString("\n  missing from .env.example: " + strings.Join(missing, ", "))
	}
	if len(stale) > 0 {
		b.WriteString("\n  stale keys in .env.example: " + strings.Join(stale, ", "))
	}
	return errors.New(b.String())
}

func exampleKeys(path string) (map[string]struct{}, error) {
	f, err := os.Open(path) //nolint:gosec // path is the caller's own .env.example
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	keys := map[string]struct{}{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if m := envKeyRe.FindStringSubmatch(strings.TrimSpace(sc.Text())); m != nil {
			keys[m[1]] = struct{}{}
		}
	}
	return keys, sc.Err()
}
