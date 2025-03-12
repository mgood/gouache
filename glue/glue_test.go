package glue_test

import (
	"strings"
	"testing"

	"github.com/mgood/gouache/glue"
	"github.com/stretchr/testify/assert"
)

func TestGlue(t *testing.T) {
	var b strings.Builder
	w := glue.NewWriter(&b)
	w.WriteString("one\ntwo\n")
	w.WriteEnd()
	assert.Equal(t, "one\ntwo\n", b.String())
}

func TestGlueInFunc(t *testing.T) {
	var b strings.Builder
	w := glue.NewWriter(&b)
	w.WriteString("before ")
	w.WriteRune(glue.FuncStart)
	w.WriteString("\n\nin-func\n\n")
	w.WriteRune(glue.FuncEnd)
	w.WriteString(" after")
	w.WriteEnd()
	assert.Equal(t, "before in-func after\n", b.String())
}

func TestSpaceAfterFuncBeginText(t *testing.T) {
	var b strings.Builder
	w := glue.NewWriter(&b)
	w.WriteRune(glue.FuncStart)
	w.WriteString("\n\n")
	w.WriteRune(glue.FuncEnd)
	w.WriteString(" after")
	w.WriteEnd()
	assert.Equal(t, "after\n", b.String())
}

func TestSpaceAfterFuncLine(t *testing.T) {
	var b strings.Builder
	w := glue.NewWriter(&b)
	w.WriteString("before\n")
	w.WriteRune(glue.FuncStart)
	w.WriteString("\n\n")
	w.WriteRune(glue.FuncEnd)
	w.WriteString(" after")
	w.WriteEnd()
	assert.Equal(t, "before\nafter\n", b.String())
}

func TestSpaceAfterFuncLine2(t *testing.T) {
	var b strings.Builder
	w := glue.NewWriter(&b)
	w.WriteString("before\n")
	w.WriteRune(glue.FuncStart)
	w.WriteRune(glue.FuncEnd)
	w.WriteString(" after")
	w.WriteEnd()
	assert.Equal(t, "before\nafter\n", b.String())
}

func TestSpaceBeforeFuncOutput(t *testing.T) {
	var b strings.Builder
	w := glue.NewWriter(&b)
	w.WriteString("before ")
	w.WriteRune(glue.FuncStart)
	w.WriteString("inside\n")
	w.WriteRune(glue.FuncEnd)
	w.WriteString(" after")
	w.WriteEnd()
	assert.Equal(t, "before inside after\n", b.String())
}

func TestSpaceBeforeFuncOutput2(t *testing.T) {
	var b strings.Builder
	w := glue.NewWriter(&b)
	w.WriteString("before ")
	w.WriteRune(glue.FuncStart)
	w.WriteString("inside\n")
	w.WriteRune(glue.FuncEnd)
	w.WriteString(", after")
	w.WriteEnd()
	assert.Equal(t, "before inside, after\n", b.String())
}

func TestNewlineAfterFunc(t *testing.T) {
	var b strings.Builder
	w := glue.NewWriter(&b)
	w.WriteString("before\n")
	w.WriteRune(glue.FuncStart)
	w.WriteString("\ninside\n")
	w.WriteRune(glue.FuncEnd)
	w.WriteString("\nafter")
	w.WriteEnd()
	assert.Equal(t, "before\ninside\nafter\n", b.String())
}

func TestImplicitInlineGlue(t *testing.T) {
	var b strings.Builder
	w := glue.NewWriter(&b)
	w.WriteString("before ")
	w.WriteRune(glue.FuncStart)
	w.WriteString("\n")
	w.WriteRune(glue.FuncEnd)
	w.WriteString("\nafter")
	w.WriteEnd()
	assert.Equal(t, "before\nafter\n", b.String())
}
