package recent

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/vehkiya/sshx/internal/sshconfig"
)

func TestRecordAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sshx", "recent.json")
	if got := Load(path); len(got) != 0 {
		t.Fatalf("Load of a missing file = %v, want empty", got)
	}

	at := time.Unix(1700000000, 0)
	if err := Record(path, "Web", at); err != nil {
		t.Fatal(err)
	}
	if err := Record(path, "db", at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	got := Load(path)
	if !got["web"].Equal(at) || !got["db"].Equal(at.Add(time.Hour)) {
		t.Errorf("Load = %v, want web at %v and db an hour later", got, at)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 && os.PathSeparator == '/' {
		t.Errorf("recent.json has mode %o, want 0600", perm)
	}
}

func TestRecordDropsOldest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recent.json")
	start := time.Unix(1700000000, 0)
	for i := range maxEntries + 1 {
		if err := Record(path, fmt.Sprintf("host%d", i), start.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	got := Load(path)
	if len(got) != maxEntries {
		t.Errorf("kept %d entries, want %d", len(got), maxEntries)
	}
	if _, ok := got["host0"]; ok {
		t.Error("the oldest entry should have been dropped")
	}
}

func TestLoadIgnoresCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recent.json")
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := Load(path); len(got) != 0 {
		t.Errorf("Load of a corrupt file = %v, want empty", got)
	}
	if err := Record(path, "web", time.Now()); err != nil {
		t.Fatalf("Record over a corrupt file: %v", err)
	}
}

func TestSort(t *testing.T) {
	hosts := []sshconfig.HostItem{{Alias: "a"}, {Alias: "B"}, {Alias: "c"}, {Alias: "d"}}
	now := time.Now()
	Sort(hosts, map[string]time.Time{"b": now.Add(-time.Hour), "d": now})
	var got []string
	for _, h := range hosts {
		got = append(got, h.Alias)
	}
	if want := []string{"d", "B", "a", "c"}; !slices.Equal(got, want) {
		t.Errorf("Sort = %q, want %q", got, want)
	}
}
