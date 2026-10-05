package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// envKeyRe matches a documented env assignment line: KEY=...
var envKeyRe = regexp.MustCompile(`^([A-Z][A-Z0-9_]*)=`)

// CheckEnvExample verifies the keys documented in the .env.example at path
// exactly match the env keys declared by Settings — both directions. It is the
// Go analogue of team-ai's scripts/check_env_example.py and is run by
// `make check-env` (via TestEnvExampleInSync).
func CheckEnvExample(path string) error {
	declared := map[string]bool{}
	for _, k := range DeclaredEnvKeys() {
		declared[k] = true
	}

	documented, err := parseEnvExampleKeys(path)
	if err != nil {
		return err
	}

	var missing, stale []string
	for k := range declared {
		if !documented[k] {
			missing = append(missing, k)
		}
	}
	for k := range documented {
		if !declared[k] {
			stale = append(stale, k)
		}
	}
	if len(missing) == 0 && len(stale) == 0 {
		return nil
	}
	sort.Strings(missing)
	sort.Strings(stale)

	var b strings.Builder
	fmt.Fprintf(&b, "%s is out of sync with internal/config Settings:\n", path)
	if len(missing) > 0 {
		b.WriteString("  missing from .env.example: " + strings.Join(missing, ", ") + "\n")
	}
	if len(stale) > 0 {
		b.WriteString("  stale keys in .env.example (no matching Settings field): " + strings.Join(stale, ", ") + "\n")
	}
	return errors.New(b.String())
}

// parseEnvExampleKeys extracts the set of KEY names from a .env.example file,
// ignoring comments and blank lines.
func parseEnvExampleKeys(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	keys := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if m := envKeyRe.FindStringSubmatch(line); m != nil {
			keys[m[1]] = true
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return keys, nil
}
