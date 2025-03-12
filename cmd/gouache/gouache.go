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
	choices, eval := Continue(w, gouache.Init(elem, listDefs), elem)
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
		fmt.Scanln(&i)
		choices, eval = Continue(w, eval, choices[i-1].Dest)
	}
	w.WriteEnd()
	b.Flush()
}

func Continue(output glue.StringWriter, eval gouache.Evaluator, elem gouache.Element) ([]gouache.Choice, gouache.Evaluator) {
	var choices []gouache.Choice
	var defaultChoice *gouache.Choice
	var s gouache.Output
	var choice *gouache.Choice
	write := func(o gouache.Output) {
		output.WriteString(o.String())
	}
	for ; ; s, choice, elem, eval = eval.Step(elem) {
		write(s)
		if choice != nil {
			if choice.IsInvisibleDefault {
				defaultChoice = choice
			} else {
				choices = append(choices, *choice)
			}
		}
		if elem != nil {
			continue
		}
		if len(choices) == 0 && defaultChoice != nil {
			elem = defaultChoice.Dest
			defaultChoice = nil
			continue
		}
		if len(choices) == 1 && defaultChoice != nil {
			elem = defaultChoice.Dest
			defaultChoice = nil
			continue
		}
		break
	}
	return choices, eval
}
