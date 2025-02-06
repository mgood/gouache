package gouache

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimpleTextOutput(t *testing.T) {
	root := Container{
		Name: "root",
		Contents: []Node{
			Text("Once upon a time..."),
			Newline{},
			Done{},
		},
	}.First()
	output, _, _ := Continue(Init(root), root)
	assert.Equal(t, "Once upon a time...\n", output)
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
	c0 := first.Find("0.c-0")
	assert.Equal(t, Text("child c-0"), c0.Node())
	g0 := c0.Find("0.g-0")
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
	elem := first.Find("$r1")
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
	elem := first.Find("1")
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
	elem = elem.Next()
	assert.Equal(t, Text("2"), elem.Node())
	elem = elem.Next()
	assert.Equal(t, Text("3"), elem.Node())
	elem = elem.Next()
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
	elem = elem.Next()
	assert.Equal(t, Text("2"), elem.Node())
	elem = elem.Next()
	assert.Equal(t, Text("3"), elem.Node())
	elem = elem.Next()
	assert.Nil(t, elem)
}

func TestSingleChoice(t *testing.T) {
	root := Container{
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
	}.First()
	output, choices, eval := Continue(Init(root), root)
	assert.Equal(t, "Once upon a time...\n", output)
	choiceNames := make([]string, len(choices))
	for i, choice := range choices {
		choiceNames[i] = choice.Label
	}
	assert.Equal(t, []string{"choice"}, choiceNames)

	output, choices, eval = Continue(eval, choices[0].Dest)
	assert.Equal(t, "The end.\n", output)
	assert.Len(t, choices, 0)
}

func Continue(eval Evaluator, elem Element) (string, []Choice, Evaluator) {
	var choices []Choice
	// TODO more general pattern for collecting output that allows access to stuff like tags
	var output strings.Builder
	var s Output
	skipNewline := true
	var choice *Choice
	for ; ; s, choice, elem, eval = eval.Step(elem) {
		switch s.String() {
		case "":
		case "\n":
			if !skipNewline {
				output.WriteString(s.String())
				skipNewline = true
			}
		default:
			output.WriteString(s.String())
			skipNewline = false
		}
		if choice != nil {
			choices = append(choices, *choice)
		}
		if elem != nil {
			continue
		}
		// TODO if we have a single default choice, we can follow that
		break
	}
	return output.String(), choices, eval
}

func load(t testing.TB, fn string) Element {
	t.Helper()
	f, err := os.Open(fn)
	assert.NoError(t, err)
	t.Cleanup(func() { f.Close() })
	el, err := LoadJSON(f)
	require.NoError(t, err)
	return el
}

func readfile(t *testing.T, fn string) string {
	t.Helper()
	b, err := os.ReadFile(fn)
	require.NoError(t, err)
	return string(b)
}

func TestSamples(t *testing.T) {
	for _, name := range []string{
		"if-else",
		"math",
		"global",
		"tempvar",
		"pop",
		"stitch",
		"sample",
	} {
		t.Run(name, func(t *testing.T) {
			base := "./testdata/" + name + ".ink"
			expected := readfile(t, base+".txt")
			root := load(t, base+".json")
			var b strings.Builder
			output, choices, eval := Continue(Init(root), root)
			b.WriteString(output)
			for len(choices) > 0 {
				b.WriteRune('\n')
				for i, choice := range choices {
					fmt.Fprintf(&b, "%d: %s\n", i+1, choice.Label)
				}
				b.WriteString("?> ")
				output, choices, eval = Continue(eval, choices[0].Dest)
				b.WriteString(output)
			}
			actual := b.String()
			assert.Equal(t, expected, actual)
		})
	}
}
