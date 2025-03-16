package main

import (
	"bufio"
	"fmt"
	"log"
	"os"

	"github.com/mgood/gouache"
	"github.com/mgood/gouache/glue"
)

func main() {
	storyPath := os.Args[1]
	f, err := os.Open(storyPath)
	if err != nil {
		log.Fatal(err)
	}
	elem, listDefs, err := gouache.LoadJSON(f)
	if err != nil {
		log.Fatal(err)
	}
	b := bufio.NewWriter(os.Stdout)
	w := glue.NewWriter(b)
	choices := gouache.Continue(w, gouache.Init(elem, listDefs), elem)
	for len(choices) > 0 {
		w.WriteEnd()
		b.WriteRune('\n')
		for i, choice := range choices {
			w.WriteString(fmt.Sprintf("%d: %s\n", i+1, choice.Label))
		}
		w.WriteEnd()
		b.WriteString("?> ")
		b.Flush()
		var i int
		if _, err := fmt.Scanln(&i); err != nil {
			log.Fatalf("unable to read input: %s", err)
		}
		choice := choices[i-1]
		choices = gouache.Continue(w, choice.Eval, choice.Dest)
	}
	w.WriteEnd()
	b.Flush()
}
