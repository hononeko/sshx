// Package completion offers shell tab completion for sshx: the scripts that
// hook it into bash, zsh and fish, and the candidates for each argument.
package completion

import (
	"fmt"
	"slices"
	"strings"

	"github.com/vehkiya/sshx/internal/recent"
	"github.com/vehkiya/sshx/internal/sshconfig"
)

// HiddenCommand is the subcommand the shell scripts call to get candidates.
const HiddenCommand = "__complete"

// Shells lists the shells Script supports.
var Shells = []string{"bash", "zsh", "fish"}

// Candidate is one completion and the description shells that support it show beside it.
type Candidate struct {
	Value, Description string
}

// String renders the candidate as the scripts read it: the value, then a tab and the description.
func (c Candidate) String() string {
	if c.Description == "" {
		return c.Value
	}
	return c.Value + "\t" + c.Description
}

// command describes a subcommand: the words that invoke it, and what its first argument completes to.
type command struct {
	names       []string
	description string
	hostArg     bool     // the first argument is a host alias
	args        []string // fixed choices for the arguments
}

var commands = []command{
	{names: []string{"add"}, description: "Add a new SSH host"},
	{names: []string{"ls", "list"}, description: "List configured SSH hosts", args: []string{"--json"}},
	{names: []string{"rm", "delete", "remove"}, description: "Remove a host", hostArg: true},
	{names: []string{"edit"}, description: "Edit a host, or open ~/.ssh/config", hostArg: true},
	{names: []string{"clone", "dup", "duplicate"}, description: "Duplicate a host", hostArg: true},
	{names: []string{"probe", "ping"}, description: "Probe a host's reachability", hostArg: true},
	{names: []string{"connect"}, description: "Pass a target straight to ssh", hostArg: true},
	{names: []string{"update", "upgrade"}, description: "Install the latest release", args: []string{"--check", "--force"}},
	{names: []string{"check-update"}, description: "Check for a newer release"},
	{names: []string{"completion"}, description: "Print a shell completion script", args: Shells},
	{names: []string{"help"}, description: "Show all commands"},
}

// Commands returns every word that invokes a subcommand, aliases included.
func Commands() []string {
	var names []string
	for _, c := range commands {
		names = append(names, c.names...)
	}
	return names
}

// Candidates returns the completions for the word being typed, the last of
// words: the arguments after the program name (an empty last word when
// nothing is typed yet). The shell narrows them down to that word. For the
// first argument they are hosts, most recently connected to first; a
// subcommand is offered only once its name is being typed, so sshx <Tab>
// lists just hosts.
func Candidates(words []string, homeDir string) []Candidate {
	cur := ""
	if len(words) > 0 {
		cur, words = words[len(words)-1], words[:len(words)-1]
	}
	if len(words) == 0 {
		out := hostCandidates(homeDir)
		if cur == "" {
			return out
		}
		for _, c := range commands {
			if strings.HasPrefix(c.names[0], strings.ToLower(cur)) {
				out = append(out, Candidate{c.names[0], c.description})
			}
		}
		return out
	}

	c, ok := lookup(words[0])
	if !ok {
		// A host alias: what follows is a remote command, which sshx can't complete.
		return nil
	}
	given := words[1:]
	switch {
	case c.hostArg && len(given) == 0:
		return hostCandidates(homeDir)
	case len(c.args) > 0 && strings.HasPrefix(c.args[0], "-"):
		// Flags may come in any order, so keep offering the ones not yet given.
		var out []Candidate
		for _, a := range c.args {
			if !slices.Contains(given, a) {
				out = append(out, Candidate{Value: a})
			}
		}
		return out
	case len(given) == 0:
		out := make([]Candidate, len(c.args))
		for i, a := range c.args {
			out[i] = Candidate{Value: a}
		}
		return out
	}
	return nil
}

func lookup(word string) (command, bool) {
	word = strings.ToLower(word)
	for _, c := range commands {
		if slices.Contains(c.names, word) {
			return c, true
		}
	}
	return command{}, false
}

// recentPath locates the connection times hosts are ordered by; tests replace it.
var recentPath = recent.Path

// hostCandidates returns every concrete alias of every configured host, each
// described by where it connects: the hosts sshx connected to most recently
// first, then the rest alphabetically.
func hostCandidates(homeDir string) []Candidate {
	hosts, err := sshconfig.LoadAllHosts(homeDir)
	if err != nil {
		return nil
	}
	if path, err := recentPath(); err == nil {
		recent.Sort(hosts, recent.Load(path))
	}
	seen := make(map[string]bool)
	var out []Candidate
	for _, h := range hosts {
		for _, a := range h.AllAliases {
			key := strings.ToLower(a)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, Candidate{a, h.Target()})
		}
	}
	return out
}

// Script returns the completion script for shell.
func Script(shell string) (string, error) {
	switch strings.ToLower(shell) {
	case "bash":
		return bashScript, nil
	case "zsh":
		return zshScript, nil
	case "fish":
		return fishScript, nil
	}
	return "", fmt.Errorf("unsupported shell %q (want one of: %s)", shell, strings.Join(Shells, ", "))
}

// The scripts pass sshx the words after its name, the word being typed last,
// and ask the shell to keep the order of what comes back where it can:
// bash 4.4+ (nosort), zsh (-V) and fish (-k). Shells or plugins that sort
// anyway list hosts alphabetically.
const bashScript = `# sshx bash completion. Load it with: source <(sshx completion bash)
_sshx() {
    local cur=${COMP_WORDS[COMP_CWORD]}
    local IFS=$'\n'
    local candidates
    candidates=$(sshx ` + HiddenCommand + ` "${COMP_WORDS[@]:1:COMP_CWORD}" 2>/dev/null | cut -f1)
    COMPREPLY=($(compgen -W "$candidates" -- "$cur"))
}
complete -o nosort -F _sshx sshx 2>/dev/null || complete -F _sshx sshx
`

const zshScript = `#compdef sshx
# sshx zsh completion. Load it with: source <(sshx completion zsh)
# or save it as _sshx in a directory on your $fpath.
_sshx() {
    local -a described
    local line name desc
    for line in "${(@f)$(sshx ` + HiddenCommand + ` "${(@)words[2,CURRENT]}" 2>/dev/null)}"; do
        [[ -n $line ]] || continue
        name=${line%%$'\t'*}
        desc=
        [[ $line == *$'\t'* ]] && desc=${line#*$'\t'}
        described+=("${name//:/\\:}:$desc")
    done
    _describe -V -t sshx 'sshx' described
}
if [[ $funcstack[1] == _sshx ]]; then
    _sshx "$@"
else
    compdef _sshx sshx
fi
`

const fishScript = `# sshx fish completion. Load it with: sshx completion fish | source
function __sshx_complete
    set -l args (commandline -opc)
    set -e args[1]
    sshx ` + HiddenCommand + ` $args (commandline -ct) 2>/dev/null
end
complete -c sshx -f -k -a '(__sshx_complete)'
`
