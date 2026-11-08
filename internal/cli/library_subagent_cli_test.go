package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/library"
)

// magpie library subagent (#5700): a subagent is added from the terminal with
// its prompt from a file, written into the agent's own folder, taken from the
// agent again when it's ticked off, and out of the library when it's removed.
func TestLibrarySubagentCLI(t *testing.T) {
	h := sandbox(t)
	write(t, filepath.Join(h, ".claude/settings.json"), "{}")
	prompt := filepath.Join(h, "prompt.md")
	write(t, prompt, "Read the diff and list what's wrong.\n")

	if err := libraryCmd([]string{"library", "subagent", "add", "reviewer", "--description=reviews a diff",
		"--prompt-file=" + prompt, "agents=claude"}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(h, ".claude/agents/reviewer.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`name: "reviewer"`, `description: "reviews a diff"`, "Read the diff and list what's wrong."} {
		if !strings.Contains(string(b), want) {
			t.Errorf("claude's subagent file has no %q:\n%s", want, b)
		}
	}
	v, err := library.Read(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Subagents) != 1 || v.Subagents[0].Name != "reviewer" || !slices.Contains(v.Subagents[0].Agents, "claude") {
		t.Fatalf("the library has %+v", v.Subagents)
	}

	// ticked off the agent: the file magpie wrote goes, the library keeps it
	if err := libraryCmd([]string{"library", "subagent", "agents", "reviewer", "none"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(h, ".claude/agents/reviewer.md")); !os.IsNotExist(err) {
		t.Errorf("claude still has the subagent: %v", err)
	}
	if v, _ = library.Read(nil); len(v.Subagents) != 1 {
		t.Errorf("the library lost a subagent it should keep: %d", len(v.Subagents))
	}

	if err := libraryCmd([]string{"library", "subagent", "rm", "reviewer"}); err != nil {
		t.Fatal(err)
	}
	if v, _ = library.Read(nil); len(v.Subagents) != 0 {
		t.Errorf("removed, and the library still has %d", len(v.Subagents))
	}
}

// A subagent with no prompt, a name agents keep for themselves, or a model
// with a space in it is refused before anything is recorded.
func TestLibrarySubagentCLIRefuses(t *testing.T) {
	h := sandbox(t)
	write(t, filepath.Join(h, ".claude/settings.json"), "{}")

	bad := map[string][]string{
		"no prompt":      {"library", "subagent", "add", "reviewer", "--description=does it"},
		"reserved name":  {"library", "subagent", "add", "default", "--description=d", "--prompt=p"},
		"model with gap": {"library", "subagent", "add", "reviewer", "--description=d", "--prompt=p", "--model=two words"},
	}
	for what, args := range bad {
		if err := libraryCmd(args); err == nil {
			t.Errorf("%s was taken", what)
		}
	}
	if v, _ := library.Read(nil); len(v.Subagents) != 0 {
		t.Errorf("a refused subagent was recorded: %+v", v.Subagents)
	}

	// and one written to no agent says how to give it to them
	if err := libraryCmd([]string{"library", "subagent", "add", "reviewer", "--description=d", "--prompt=p"}); err != nil {
		t.Fatal(err)
	}
	if v, _ := library.Read(nil); len(v.Subagents) != 1 || len(v.Subagents[0].Agents) != 0 {
		t.Fatalf("a subagent with no agents= is given to %v", v.Subagents)
	}
}
