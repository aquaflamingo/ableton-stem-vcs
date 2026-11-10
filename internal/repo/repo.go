// Package repo implements the on-disk layout of a Stem-managed Live project:
// adopting an existing project folder (init) and rebuilding the working .als
// from the canonical tree (rebuild).
package repo

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aquaflamingo/stem/internal/als"
	"github.com/aquaflamingo/stem/internal/canon"
)

const (
	configFile = "stem.toml"
	setDir     = "set"
	localDir   = ".stem"
)

// Config is the decoded stem.toml (a small hand-written subset of TOML).
type Config struct {
	Name           string
	SetFile        string
	MinLiveVersion string
}

// Init adopts dir as a Stem repository: it explodes the working .als into the
// canonical set/ tree, writes stem.toml, .gitignore and .gitattributes, and
// initialises a git repository. It returns the Set file it adopted.
func Init(dir string) (string, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	setFile, err := findSetFile(absDir)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(absDir, configFile)); err == nil {
		return "", fmt.Errorf("repo: %s already contains %s", absDir, configFile)
	}

	doc, err := als.Load(setFile)
	if err != nil {
		return "", err
	}
	if _, err := canon.Explode(doc, filepath.Join(absDir, setDir)); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(absDir, localDir), 0o755); err != nil {
		return "", err
	}

	name := strings.TrimSuffix(filepath.Base(setFile), filepath.Ext(setFile))
	if name == "" || name == "." {
		name = filepath.Base(absDir)
	}
	cfg := &Config{Name: name, SetFile: filepath.Base(setFile), MinLiveVersion: minorVersion(doc)}
	if err := writeConfig(absDir, cfg); err != nil {
		return "", err
	}
	if err := writeIfAbsent(filepath.Join(absDir, ".gitignore"), gitignore(cfg)); err != nil {
		return "", err
	}
	if err := writeIfAbsent(filepath.Join(absDir, ".gitattributes"), gitattributes); err != nil {
		return "", err
	}

	if err := gitInitAdd(absDir); err != nil {
		return setFile, err
	}
	return setFile, nil
}

// Rebuild regenerates the working .als from the canonical set/ tree.
func Rebuild(dir string) (string, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	cfg, err := LoadConfig(absDir)
	if err != nil {
		return "", err
	}
	set, err := canon.LoadSet(filepath.Join(absDir, setDir))
	if err != nil {
		return "", err
	}
	out := filepath.Join(absDir, cfg.SetFile)
	if err := set.ImplodeToFile(filepath.Join(absDir, setDir), out); err != nil {
		return "", err
	}
	return out, nil
}

// LoadConfig reads stem.toml.
func LoadConfig(dir string) (*Config, error) {
	b, err := os.ReadFile(filepath.Join(dir, configFile))
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		switch k {
		case "name":
			cfg.Name = v
		case "set_file":
			cfg.SetFile = v
		case "min_live_version":
			cfg.MinLiveVersion = v
		}
	}
	if cfg.SetFile == "" {
		return nil, fmt.Errorf("repo: %s: missing set_file", configFile)
	}
	return cfg, nil
}

func writeConfig(dir string, cfg *Config) error {
	var b strings.Builder
	b.WriteString("[project]\n")
	fmt.Fprintf(&b, "name = %q\n", cfg.Name)
	fmt.Fprintf(&b, "set_file = %q\n", cfg.SetFile)
	fmt.Fprintf(&b, "min_live_version = %q\n", cfg.MinLiveVersion)
	b.WriteString("\n[assets]\nexclude = [\"**/*.asd\", \"Backup\"]\n")
	return os.WriteFile(filepath.Join(dir, configFile), []byte(b.String()), 0o644)
}

func findSetFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var found []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".als") {
			found = append(found, filepath.Join(dir, e.Name()))
		}
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("repo: no .als file found in %s", dir)
	case 1:
		return found[0], nil
	default:
		sort.Strings(found)
		return "", fmt.Errorf("repo: multiple .als files in %s; remove all but one:\n  %s",
			dir, strings.Join(found, "\n  "))
	}
}

func minorVersion(doc *als.Doc) string {
	if v, ok := doc.Root.Attr("MinorVersion"); ok && v != "" {
		if i := strings.IndexByte(v, '_'); i > 0 {
			return v[:i]
		}
		return v
	}
	return ""
}

func gitignore(cfg *Config) string {
	return fmt.Sprintf(`# Live Set is a build artifact; rebuild with: stem rebuild
%s
Backup/
*.asd
*.alp
.stem/
`, cfg.SetFile)
}

const gitattributes = `set/** text
set/**/*.state -text
`

func writeIfAbsent(path, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func gitInitAdd(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		if out, err := run(dir, "init", "-b", "main"); err != nil {
			// older git may not support -b
			if _, err2 := run(dir, "init"); err2 != nil {
				return fmt.Errorf("git init: %v: %s", err, out)
			}
		}
	}
	if _, err := run(dir, "add", "-A"); err != nil {
		return fmt.Errorf("git add failed")
	}
	return nil
}

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
