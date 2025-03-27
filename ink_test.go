package gouache

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/mgood/gouache/glue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimpleTextOutput(t *testing.T) {
	c := Container{
		Name: "root",
		Contents: []Node{
			Text("Once upon a time..."),
			Newline{},
			Done{},
		},
	}
	var b strings.Builder
	w := glue.NewWriter(&b)
	root, eval := Init(c, nil)
	ContinueT(t, w, eval, root)
	w.WriteEnd()
	assert.Equal(t, "Once upon a time...\n", b.String())
}

func TestSplitAddress(t *testing.T) {
	pre, post := splitAddress("a.b")
	assert.Equal(t, []string{"a"}, pre)
	assert.Equal(t, "b", post)

	pre, post = splitAddress("a")
	assert.Len(t, pre, 0)
	assert.Equal(t, "a", post)
}

func TestLookupNestedFullAddress(t *testing.T) {
	root := Container{
		Name: "root",
		Contents: []Node{
			Container{
				Contents: []Node{
					Text("root 0"),
				},
				Nested: map[string]Container{
					"c-0": {
						Name: "c-0",
						Contents: []Node{
							Text("child c-0"),
						},
					},
					"g-0": {
						Name: "g-0",
						Contents: []Node{
							Text("child g-0"),
						},
					},
				},
			},
		},
	}
	first := root.First()
	c0, _ := first.Find("0.c-0")
	assert.Equal(t, Text("child c-0"), c0.Node())
	g0, _ := c0.Find("0.g-0")
	assert.Equal(t, Text("child g-0"), g0.Node())
}

func TestLookupNamedContentElement(t *testing.T) {
	root := Container{
		Name: "root",
		Contents: []Node{
			Text("1"),
			Container{
				Name: "$r1",
				Contents: []Node{
					Text("2"),
				},
			},
			Text("3"),
		},
	}
	first := root.First()
	elem, _ := first.Find("$r1")
	assert.Equal(t, Text("2"), elem.Node())
}

func TestLookupIndex(t *testing.T) {
	root := Container{
		Name: "root",
		Contents: []Node{
			Text("root 0"),
			Text("root 1"),
			Text("root 2"),
		},
	}
	first := root.First()
	elem, _ := first.Find("1")
	assert.Equal(t, Text("root 1"), elem.Node())
}

func TestContainerElementContinuation(t *testing.T) {
	root := Container{
		Name: "root",
		Contents: []Node{
			Text("1"),
			Container{
				Contents: []Node{
					Text("2"),
				},
			},
			Text("3"),
		},
	}
	elem := root.First()
	assert.Equal(t, Text("1"), elem.Node())
	elem, _ = elem.Next()
	assert.Equal(t, Text("2"), elem.Node())
	elem, _ = elem.Next()
	assert.Equal(t, Text("3"), elem.Node())
	elem, _ = elem.Next()
	assert.Nil(t, elem)
}

func TestContainerFirstElementContinuation(t *testing.T) {
	root := Container{
		Name: "root",
		Contents: []Node{
			Container{
				Contents: []Node{
					Text("1"),
				},
			},
			Text("2"),
			Text("3"),
		},
	}
	elem := root.First()
	assert.Equal(t, Text("1"), elem.Node())
	elem, _ = elem.Next()
	assert.Equal(t, Text("2"), elem.Node())
	elem, _ = elem.Next()
	assert.Equal(t, Text("3"), elem.Node())
	elem, _ = elem.Next()
	assert.Nil(t, elem)
}

func TestSingleChoice(t *testing.T) {
	c := Container{
		Name: "root",
		Contents: []Node{
			Container{
				Contents: []Node{
					Text("Once upon a time..."),
					Newline{},
					BeginEval{},
					BeginStringEval{},
					Text("choice"),
					EndStringEval{},
					EndEval{},
					ChoicePoint{Dest: "0.c-0", Flags: 20},
				},
				Nested: map[string]Container{
					"c-0": {
						Name: "c-0",
						Contents: []Node{
							Text("The end."),
							Newline{},
							Done{},
						},
					},
				},
			},
			Done{},
		},
	}
	var b strings.Builder
	w := glue.NewWriter(&b)
	root, eval := Init(c, nil)
	choices := ContinueT(t, w, eval, root)
	w.WriteEnd()
	assert.Equal(t, "Once upon a time...\n", b.String())
	choiceNames := make([]string, len(choices))
	for i, choice := range choices {
		choiceNames[i] = choice.Label
	}
	assert.Equal(t, []string{"choice"}, choiceNames)

	b.Reset()
	choice := choices[0]
	choices = ContinueT(t, w, choice.Eval, choice.Dest)
	w.WriteEnd()
	assert.Equal(t, "The end.\n", b.String())
	assert.Len(t, choices, 0)
}

func elementString(elem Element) string {
	if elem == nil {
		return "<nil>"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "{")
	addr, i := elem.Address()
	fmt.Fprintf(&b, "%q %d %#v", addr, i, elem.Node())
	fmt.Fprintf(&b, "}")
	return b.String()
}

type tLogger interface {
	Logf(string, ...interface{})
}

type loggingEvaluator struct {
	TB       TBMinimal
	Eval     Evaluator
	MaxSteps int
}

func logEval(t TBMinimal, eval Evaluator, maxSteps int) Evaluator {
	return loggingEvaluator{TB: t, Eval: eval, MaxSteps: maxSteps}
}

func (e loggingEvaluator) Step(elem Element) (Output, *Choice, Element, Evaluator) {
	if e.MaxSteps <= 0 {
		e.TB.Errorf("max steps exceeded")
		e.TB.FailNow()
	}
	e.TB.Logf("%T %s", e, elementString(elem))
	out, choice, elem, eval := e.Eval.Step(elem)
	return out, choice, elem, logEval(e.TB, eval, e.MaxSteps-1)
}

func ContinueT(t TBMinimal, output glue.StringWriter, eval Evaluator, elem Element) []Choice {
	return Continue(output, logEval(t, eval, 10000), elem)
}

type TBMinimal interface {
	Helper()
	Cleanup(func())
	Logf(string, ...interface{})
	require.TestingT
	assert.TestingT
}

func load(t TBMinimal, fn string) (Container, ListDefs) {
	t.Helper()
	f, err := os.Open(fn)
	assert.NoError(t, err)
	t.Cleanup(func() { f.Close() })
	el, listDefs, err := LoadJSON(f)
	require.NoError(t, err)
	return el, listDefs
}

func readfile(t TBMinimal, fn string) string {
	t.Helper()
	b, err := os.ReadFile(fn)
	require.NoError(t, err)
	if len(b) == 0 {
		return "\n"
	}
	return string(b)
}

type stringWriteFunc func(string) (int, error)

func (f stringWriteFunc) WriteString(s string) (int, error) { return f(s) }

func TestSamples(t *testing.T) {
	for _, name := range []string{
		"choice-condition",
		"choice-count",
		"choice-func-content",
		"func-abs",
		"func-text-content",
		"func-return-eval",
		"global",
		"glue",
		"if-else",
		"math",
		"math-type-coercion",
		"list-basics",
		"pop",
		"random",
		"random-shuffle",
		"random-shuffle-text",
		"sample",
		"seq-text",
		"stitch",
		"tempvar",
		"threads",
		"tunnels",
		"turn-count",
		"var-ref",
		"visit-count",
	} {
		t.Run(name, func(t *testing.T) {
			base := "./testdata/" + name + ".ink"
			expected := readfile(t, base+".txt")
			container, listDefs := load(t, base+".json")
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
				choice := choices[0]
				choices = ContinueT(t, write, choice.Eval, choice.Dest)
			}
			w.WriteEnd()
			actual := b.String()
			assert.Equal(t, expected, actual)
		})
	}
}
