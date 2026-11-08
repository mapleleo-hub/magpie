package cli

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/library"
)

// sandbox is a temp HOME whose agents are the folders written under it, with
// the library's own folder inside it too.
func sandbox(t *testing.T) string {
	t.Helper()
	h := t.TempDir()
	for k, v := range map[string]string{"HOME": h, "USERPROFILE": h, "XDG_CONFIG_HOME": filepath.Join(h, ".config"), "CLAUDE_CONFIG_DIR": "", "CODEX_HOME": "", "APPDATA": "", "LOCALAPPDATA": ""} {
		t.Setenv(k, v)
	}
	return h
}

// write puts a file, and the folders above it, down.
func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// readOut is what printing something to stdout prints.
func readOut(t *testing.T, f func() error) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	if err := f(); err != nil {
		os.Stdout = old
		t.Fatal(err)
	}
	os.Stdout = old
	w.Close()
	return <-done
}

// magpie library hook (#5700): a hook is added from the terminal and written
// into the agent's own file in its own shape, taken from the agent again when
// it's ticked off, and out of the library when it's removed.
func TestLibraryHookCLI(t *testing.T) {
	h := sandbox(t)
	write(t, filepath.Join(h, ".claude/settings.json"), "{}")

	if err := libraryCmd([]string{"library", "hook", "add", "notify", "turn-end", "echo done", "agents=claude"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(h, ".claude/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Claude Code's own name for turn-end is Stop
	if !strings.Contains(string(b), `"Stop"`) || !strings.Contains(string(b), "echo done") {
		t.Fatalf("the hook isn't in claude's settings where its Stop event is: %s", b)
	}
	v, err := library.Read(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Hooks) != 1 || v.Hooks[0].Name != "notify" || v.Hooks[0].Event != "turn-end" || !slices.Contains(v.Hooks[0].Agents, "claude") {
		t.Fatalf("the library has %+v", v.Hooks)
	}

	// a command with flags of its own is everything after --
	if err := libraryCmd([]string{"library", "hook", "add", "fmt", "after-tool", "--", "prettier", "--write", "."}); err != nil {
		t.Fatal(err)
	}
	v, err = library.Read(nil)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(v.Hooks, func(x library.HookView) bool { return x.Name == "fmt" })
	if i < 0 || v.Hooks[i].Command != "prettier --write ." {
		t.Fatalf("the command after -- came out as %q", v.Hooks[i].Command)
	}
	if len(v.Hooks[i].Agents) != 0 {
		t.Errorf("a hook added with no agents= is given to %v", v.Hooks[i].Agents)
	}

	// ticked off the agent: what magpie wrote goes, the library keeps it
	if err := libraryCmd([]string{"library", "hook", "agents", "notify", "none"}); err != nil {
		t.Fatal(err)
	}
	b, err = os.ReadFile(filepath.Join(h, ".claude/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "echo done") {
		t.Errorf("claude still has the hook: %s", b)
	}
	if v, _ = library.Read(nil); len(v.Hooks) != 2 {
		t.Errorf("the library lost a hook it should keep: %d", len(v.Hooks))
	}

	if err := libraryCmd([]string{"library", "hook", "rm", "notify", "fmt"}); err != nil {
		t.Fatal(err)
	}
	if v, _ = library.Read(nil); len(v.Hooks) != 0 {
		t.Errorf("removed, and the library still has %d", len(v.Hooks))
	}
}

// A hook for an agent magpie gives none, or at an event it doesn't know, is
// refused before anything is recorded.
func TestLibraryHookCLIRefuses(t *testing.T) {
	h := sandbox(t)
	write(t, filepath.Join(h, ".claude/settings.json"), "{}")
	write(t, filepath.Join(h, ".config/opencode/opencode.json"), "{}")

	err := libraryCmd([]string{"library", "hook", "add", "x", "prompt", "echo hi", "agents=opencode"})
	if err == nil || !strings.Contains(err.Error(), "no hooks") {
		t.Errorf("a hook for an agent that takes none: %v", err)
	}
	err = libraryCmd([]string{"library", "hook", "add", "x", "nope", "echo hi", "agents=claude"})
	if err == nil || !strings.Contains(err.Error(), "no event called nope") {
		t.Errorf("a hook at no event: %v", err)
	}
	err = libraryCmd([]string{"library", "hook", "add", "x", "prompt"})
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Errorf("a hook with no command: %v", err)
	}
	if v, _ := library.Read(nil); len(v.Hooks) != 0 {
		t.Errorf("a refused hook was recorded: %+v", v.Hooks)
	}
}

// magpie library lists the hooks and subagents it holds, as it lists the
// instructions, servers and skills.
func TestLibraryStatusListsHooksAndSubagents(t *testing.T) {
	h := sandbox(t)
	write(t, filepath.Join(h, ".claude/settings.json"), "{}")

	if err := libraryCmd([]string{"library", "hook", "add", "notify", "prompt", "echo hi", "agents=claude"}); err != nil {
		t.Fatal(err)
	}
	out := readOut(t, func() error { return libraryCmd([]string{"library"}) })
	for _, want := range []string{"Hooks", "notify", "Subagents"} {
		if !strings.Contains(out, want) {
			t.Errorf("magpie library printed no %s:\n%s", want, out)
		}
	}
}
