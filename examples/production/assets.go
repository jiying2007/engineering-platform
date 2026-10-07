// Package productionassets embeds only the canonical non-secret deployment
// examples. These same source files remain authoritative; no generated copy is
// maintained. Installing them does not configure or enable any service.
package productionassets

import (
	"embed"
	"io/fs"
)

//go:embed README.md *.example *.tmpl systemd/*.service systemd/maintenance/*.service systemd/maintenance/*.timer apparmor/engineering-worker terminal-maintenance/*.md terminal-maintenance/*.json terminal-maintenance/*.tmpl terminal-maintenance/*.txt
var files embed.FS

func Files() (map[string][]byte, error) {
	out := make(map[string][]byte)
	err := fs.WalkDir(files, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := files.ReadFile(path)
		if err == nil {
			out[path] = raw
		}
		return err
	})
	return out, err
}
