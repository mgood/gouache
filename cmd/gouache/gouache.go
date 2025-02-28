package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/mgood/gouache"
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
	output, choices, eval := Continue(gouache.Init(elem, listDefs), elem)
	fmt.Print(output)
	for len(choices) > 0 {
		fmt.Println()
		for i, choice := range choices {
			fmt.Printf("%d: %s\n", i+1, choice.Label)
		}
		fmt.Print("?> ")
		var i int
		fmt.Scanln(&i)
		output, choices, eval = Continue(eval, choices[i-1].Dest)
		fmt.Print(output)
	}
}

func Continue(eval gouache.Evaluator, elem gouache.Element) (string, []gouache.Choice, gouache.Evaluator) {
	var choices []gouache.Choice
	var defaultChoice *gouache.Choice
	var output strings.Builder
	var s gouache.Output
	skipNewline := true
	var choice *gouache.Choice
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
	return output.String(), choices, eval
}
