package completion

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/hononeko/sshx/internal/recent"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	useRecent(t, filepath.Join(home, "recent.json"))
	return home
}

// useRecent points completion at path for connection times, never the user's own.
func useRecent(t *testing.T, path string) {
	t.Helper()
	old := recentPath
	recentPath = func() (string, error) { return path, nil }
	t.Cleanup(func() { recentPath = old })
}

func values(cs []Candidate) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Value
	}
	return out
}

const config = `Host *
    ServerAliveInterval 30

Host web web.lan !bad
    HostName 10.0.0.5
    User deploy
    Port 2222

Host db
    HostName db.internal

Host WEB
    HostName duplicate
`

func TestCandidates(t *testing.T) {
	home := writeConfig(t, config)
	hosts := []string{"db", "web", "web.lan"}

	tests := []struct {
		words []string
		want  []string
	}{
		{nil, hosts},
		{[]string{""}, hosts},
		{[]string{"w"}, hosts},
		{[]string{"e"}, append(hosts, "edit")},
		{[]string{"he"}, append(hosts, "help")},
		{[]string{"help", ""}, nil},
		{[]string{"C"}, append(hosts, "clone", "connect", "check-update", "completion")},
		{[]string{"rm", ""}, hosts},
		{[]string{"EDIT", "w"}, hosts},
		{[]string{"connect", ""}, hosts},
		{[]string{"rm", "web", ""}, nil},
		{[]string{"web", ""}, nil},
		{[]string{"web", "uptime", ""}, nil},
		{[]string{"add", ""}, nil},
		{[]string{"completion", ""}, Shells},
		{[]string{"completion", "zsh", ""}, nil},
		{[]string{"ls", "-"}, []string{"--json"}},
		{[]string{"ls", "--json", ""}, nil},
		{[]string{"update", ""}, []string{"--check", "--force"}},
		{[]string{"upgrade", "--force", ""}, []string{"--check"}},
	}
	for _, tt := range tests {
		if got := values(Candidates(tt.words, home)); !slices.Equal(got, tt.want) {
			t.Errorf("Candidates(%q) = %q, want %q", tt.words, got, tt.want)
		}
	}
}

func TestCandidatesHosts(t *testing.T) {
	home := writeConfig(t, config)
	got := Candidates([]string{""}, home)
	vals := values(got)
	for _, unwanted := range []string{"*", "!bad", "WEB", "add", "list", HiddenCommand} {
		if slices.Contains(vals, unwanted) {
			t.Errorf("sshx <Tab> candidates %q include %q", vals, unwanted)
		}
	}
	for _, c := range got {
		if c.Value == "web.lan" && c.Description != "deploy@10.0.0.5:2222" {
			t.Errorf("web.lan described as %q, want its target", c.Description)
		}
	}
}

func TestCandidatesRecentFirst(t *testing.T) {
	home := writeConfig(t, config)
	path, _ := recentPath()
	now := time.Now()
	if err := recent.Record(path, "web", now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := recent.Record(path, "db", now); err != nil {
		t.Fatal(err)
	}
	if got, want := values(Candidates([]string{""}, home)), []string{"db", "web", "web.lan"}; !slices.Equal(got, want) {
		t.Errorf("after connecting to db: %q, want %q", got, want)
	}
	if err := recent.Record(path, "web", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got, want := values(Candidates([]string{"rm", ""}, home)), []string{"web", "web.lan", "db"}; !slices.Equal(got, want) {
		t.Errorf("after connecting to web: %q, want %q", got, want)
	}
}

func TestCandidatesWithoutConfig(t *testing.T) {
	useRecent(t, filepath.Join(t.TempDir(), "recent.json"))
	if got := Candidates([]string{"rm", ""}, t.TempDir()); len(got) != 0 {
		t.Errorf("Candidates without a config = %q, want none", values(got))
	}
}

func TestCandidateString(t *testing.T) {
	if got := (Candidate{"web", "deploy@web"}).String(); got != "web\tdeploy@web" {
		t.Errorf("String() = %q", got)
	}
	if got := (Candidate{Value: "--json"}).String(); got != "--json" {
		t.Errorf("String() = %q", got)
	}
}

func TestScript(t *testing.T) {
	for _, shell := range Shells {
		s, err := Script(shell)
		if err != nil || s == "" {
			t.Errorf("Script(%q) = %q, %v", shell, s, err)
		}
	}
	if _, err := Script("powershell"); err == nil {
		t.Error("Script(powershell) should fail")
	}
}
