// Package recent remembers when sshx last connected to each host, so hosts
// can be offered most recently used first.
package recent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hononeko/sshx/internal/sshconfig"
)

// maxEntries bounds the file, dropping the hosts used longest ago.
const maxEntries = 500

// Path returns the file sshx keeps its connection times in, beside the update check's cache.
func Path() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sshx", "recent.json"), nil
}

// Load returns when each host was last connected to, keyed by its lower-cased
// primary alias. A missing or unreadable file gives an empty map.
func Load(path string) map[string]time.Time {
	out := make(map[string]time.Time)
	data, err := os.ReadFile(filepath.Clean(path)) //nolint:gosec // sshx's own cache file
	if err != nil {
		return out
	}
	var stamps map[string]int64
	if json.Unmarshal(data, &stamps) != nil {
		return out
	}
	for alias, unix := range stamps {
		out[alias] = time.Unix(unix, 0)
	}
	return out
}

// Record notes that alias was connected to at now.
func Record(path, alias string, now time.Time) error {
	times := Load(path)
	times[strings.ToLower(alias)] = now

	aliases := make([]string, 0, len(times))
	for a := range times {
		aliases = append(aliases, a)
	}
	sort.Slice(aliases, func(i, j int) bool { return times[aliases[i]].After(times[aliases[j]]) })
	if len(aliases) > maxEntries {
		aliases = aliases[:maxEntries]
	}

	stamps := make(map[string]int64, len(aliases))
	for _, a := range aliases {
		stamps[a] = times[a].Unix()
	}
	data, err := json.Marshal(stamps)
	if err != nil {
		return err
	}
	return sshconfig.AtomicWrite(path, data, 0600)
}

// Sort orders hosts most recently connected to first; hosts never connected
// to keep their order after them.
func Sort(hosts []sshconfig.HostItem, times map[string]time.Time) {
	sort.SliceStable(hosts, func(i, j int) bool {
		ti, tj := times[strings.ToLower(hosts[i].Alias)], times[strings.ToLower(hosts[j].Alias)]
		return ti.After(tj)
	})
}
