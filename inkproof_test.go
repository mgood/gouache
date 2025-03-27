package gouache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mgood/gouache/glue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	bytecode = regexp.MustCompile(`^B\d+$`)
	ink      = regexp.MustCompile(`^I\d+$`)
)

func TestInkProofBytecode(t *testing.T) {
	root := "./testdata/ink-proof/bytecode"
	contents, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		t.Skipf("missing test files in %q", root)
	}
	require.NoError(t, err)
	for _, entry := range contents {
		name := entry.Name()
		if !bytecode.MatchString(name) {
			continue
		}
		t.Run(name, func(t *testing.T) {
			base := filepath.Join(root, name)
			expected := readfile(t, filepath.Join(base, "transcript.txt"))
			input := openfile(t, filepath.Join(base, "input.txt"))
			container, listDefs := load(t, filepath.Join(base, "bytecode.json"))
			var b strings.Builder
			w := glue.NewWriter(&b)
			root, eval := Init(container, listDefs)
			choices := ContinueT(t, w, eval, root)
			for len(choices) > 0 {
				w.WriteEnd()
				b.WriteRune('\n')
				for i, choice := range choices {
					b.WriteString(fmt.Sprintf("%d: %s\n", i+1, choice.Label))
				}
				b.WriteString("?> ")
				var choiceNum int
				fmt.Fscanln(input, &choiceNum)
				choice := choices[choiceNum-1]
				choices = ContinueT(t, w, choice.Eval, choice.Dest)
			}
			w.WriteEnd()
			actual := b.String()
			assert.Equal(t, expected, actual)
		})
	}
}

func openfile(t TBMinimal, fn string) io.Reader {
	t.Helper()
	b, err := os.Open(fn)
	require.NoError(t, err)
	t.Cleanup(func() { b.Close() })
	return b
}

func readjson[T any](t *testing.T, fn string) T {
	t.Helper()
	f := openfile(t, fn)
	var v T
	require.NoError(t, json.NewDecoder(f).Decode(&v))
	return v
}

func TestInkProofInk(t *testing.T) {
	runOnly := ""
	skipReason := func(name string) string {
		if runOnly != "" {
			if name == runOnly {
				return ""
			}
			return fmt.Sprintf("only running %s", runOnly)
		}
		return map[string]string{
			"I059": "tunnel choice stack",
			"I066": "tunnel self timeout",
			"I098": "knot & thread interaction",
			"I099": "tags",
			"I100": "tags",
			"I104": "thread newline?",
			"I128": "visit counts",
			"I130": "knots & thread interaction",
		}[name]
	}

	root := "./testdata/ink-proof/ink"
	contents, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		t.Skipf("missing test files in %q", root)
	}
	require.NoError(t, err)
	for _, entry := range contents {
		name := entry.Name()
		if !ink.MatchString(name) {
			continue
		}
		base := filepath.Join(root, name)
		meta := readjson[struct {
			Description string `json:"oneLineDescription"`
			Hide        any    `json:"hide"`
		}](t, filepath.Join(base, "metadata.json"))
		t.Run(fmt.Sprintf("%s %s", name, meta.Description), func(t *testing.T) {
			if meta.Hide != nil {
				t.Skipf("hidden by metadata.json: %v", meta.Hide)
			}
			if reason := skipReason(name); reason != "" {
				t.Skipf("%s: %s", name, reason)
			}
			expected := readfile(t, filepath.Join(base, "transcript.txt"))
			input := openfile(t, filepath.Join(base, "input.txt"))
			container, listDefs := load(t, filepath.Join(base, "story.ink.json"))
			var b strings.Builder
			w := glue.NewWriter(&b)
			write := stringWriteFunc(func(s string) (int, error) {
				t.Logf("%q", s)
				return w.WriteString(s)
			})
			root, eval := Init(container, listDefs)
			choices := ContinueT(t, write, eval, root)
			for len(choices) > 0 {
				w.WriteEnd()
				b.WriteRune('\n')
				for i, choice := range choices {
					write(fmt.Sprintf("%d: %s\n", i+1, choice.Label))
				}
				w.WriteEnd()
				b.WriteString("?> ")
				var choiceNum int
				if _, err := fmt.Fscanln(input, &choiceNum); err != nil {
					t.Fatalf("unable to read choice input: %s", err)
				}
				choice := choices[choiceNum-1]
				choices = ContinueT(t, write, choice.Eval, choice.Dest)
			}
			w.WriteEnd()
			actual := b.String()
			if !strings.HasSuffix(actual, "\n") {
				actual += "\n"
			}
			assert.Equal(t, expected, actual)
		})
	}
}
