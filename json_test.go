package gouache

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadJSON(t *testing.T) {
	f, err := os.Open("./testdata/sample.ink.json")
	assert.NoError(t, err)
	t.Cleanup(func() { f.Close() })
	c, err := LoadJSON(f)
	assert.NoError(t, err)
	assert.Equal(t, Text("Once upon a time..."), c.Node())
	el := c.Find("0.c-0")
	assert.Equal(t, BeginEval{}, el.Node())
}
