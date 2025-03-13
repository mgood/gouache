package glue

import "strings"

type RuneWriter interface {
	WriteRune(rune) (n int, err error)
}

type StringWriter interface {
	WriteString(string) (n int, err error)
}

type RuneStringWriter interface {
	RuneWriter
	StringWriter
	WriteEnd() (n int, err error)
}

type writer struct {
	RuneWriter
	state stateFn
}

func NewWriter(b RuneWriter) RuneStringWriter {
	return &writer{b, stateBeginText}
}

func (w *writer) WriteEnd() (n int, err error) {
	return w.WriteRune(StreamEnd)
}

func (w *writer) WriteString(s string) (n int, err error) {
	return writeRunes(w, []rune(s)...)
}

func (w *writer) WriteRune(r rune) (n int, err error) {
	w.state, n, err = w.state(w.RuneWriter, r)
	return
}

func StripInline(s string) string {
	var b strings.Builder
	w := NewWriter(&b)
	// in order to preserve surrounding spaces, add non-space characters
	// to the beginning and end of the string, and then strip them after
	w.WriteRune('^')
	w.WriteString(s)
	w.WriteRune('$')
	return strings.TrimPrefix(strings.TrimSuffix(b.String(), "$"), "^")
}

const (
	// Use U+2060 WORD JOINER to prevent line breaks at this point. Will need
	// the output handler to look for this and combine lines, but seems like
	// this should be a useful way to detect glue.
	Glue = '\u2060'

	// Use control codes to mark the start & end of functions.
	// Using "Shift Out" and "Shift In" somewhat arbitrarily since they're
	// paired, but just need something that would not be valid in the output.
	FuncStart = '\u000e'
	FuncEnd   = '\u000f'

	// Use NUL byte to mark the end of a stream of text. This is used by
	// WriteEnd to put a '\n' if needed after a block of text. This mainly
	// resets the state before presenting something like a choice that would
	// force a new line.
	StreamEnd = '\u0000'
)

type stateFn func(b RuneWriter, r rune) (stateFn, int, error)

func stateBeginText(b RuneWriter, r rune) (stateFn, int, error) {
	switch r {
	case '\n', ' ':
		return stateBeginText, 0, nil
	case FuncStart:
		return stateFuncStartBeginText, 0, nil
	case FuncEnd, StreamEnd:
		return stateBeginText, 0, nil
	case Glue:
		return stateGlue, 0, nil
	default:
		n, err := b.WriteRune(r)
		return stateInWord, n, err
	}
}

func stateFuncStartBeginText(b RuneWriter, r rune) (stateFn, int, error) {
	switch r {
	case '\n', ' ':
		return stateBeginText, 0, nil
	case FuncStart:
		return stateFuncStartBeginText, 0, nil
	case FuncEnd, StreamEnd:
		return stateBeginText, 0, nil
	case Glue:
		return stateGlue, 0, nil
	default:
		return next(stateInWord, b, r)
	}
}

func stateBeginLine(b RuneWriter, r rune) (stateFn, int, error) {
	switch r {
	case '\n', ' ':
		return stateBeginLine, 0, nil
	case FuncStart:
		return stateFuncStartBeginLine, 0, nil
	case FuncEnd:
		return stateInWord, 0, nil
	case Glue:
		return stateGlue, 0, nil
	case StreamEnd:
		return next(stateBeginText, b, '\n')
	default:
		return next(stateInWord, b, '\n', r)
	}
}

func stateFuncStartBeginLine(b RuneWriter, r rune) (stateFn, int, error) {
	switch r {
	case '\n', ' ':
		return stateFuncStartBeginLine, 0, nil
	case FuncStart:
		return stateFuncStartBeginLine, 0, nil
	case FuncEnd:
		return stateBeginLine, 0, nil
	case Glue:
		return stateGlue, 0, nil
	case StreamEnd:
		return next(stateBeginText, b, '\n')
	default:
		return next(stateInWord, b, '\n', r)
	}
}

func stateFuncStartInWord(b RuneWriter, r rune) (stateFn, int, error) {
	switch r {
	case '\n':
		return stateInWord, 0, nil
	case ' ':
		return stateSpaces, 0, nil
	case FuncStart:
		return stateFuncStartInWord, 0, nil
	case FuncEnd:
		return stateInWord, 0, nil
	case Glue:
		return stateGlue, 0, nil
	case StreamEnd:
		return next(stateBeginText, b, '\n')
	default:
		return next(stateInWord, b, r)
	}
}

func stateFuncStartSpace(b RuneWriter, r rune) (stateFn, int, error) {
	switch r {
	case '\n', ' ':
		return stateFuncStartSpace, 0, nil
	case FuncStart:
		return stateFuncStartSpace, 0, nil
	case FuncEnd:
		return stateSpaces, 0, nil
	case Glue:
		return stateGlueSpace, 0, nil
	case StreamEnd:
		return next(stateBeginText, b, '\n')
	default:
		return next(stateInWord, b, ' ', r)
	}
}

func stateGlue(b RuneWriter, r rune) (stateFn, int, error) {
	switch r {
	case ' ':
		return stateGlueSpace, 0, nil
	case '\n', Glue, FuncStart, FuncEnd:
		return stateGlue, 0, nil
	case StreamEnd:
		return next(stateBeginText, b, '\n')
	default:
		return next(stateInWord, b, r)
	}
}

func stateGlueSpace(b RuneWriter, r rune) (stateFn, int, error) {
	switch r {
	case ' ', '\n', Glue, FuncStart, FuncEnd:
		return stateGlueSpace, 0, nil
	case StreamEnd:
		return next(stateBeginText, b, '\n')
	default:
		return next(stateInWord, b, ' ', r)
	}
}

func stateInWord(b RuneWriter, r rune) (stateFn, int, error) {
	switch r {
	case ' ':
		return stateSpaces, 0, nil
	case '\n':
		return stateBeginLine, 0, nil
	case FuncStart:
		return stateFuncStartInWord, 0, nil
	case FuncEnd:
		return stateInWord, 0, nil
	case Glue:
		return stateGlue, 0, nil
	case StreamEnd:
		return next(stateBeginText, b, '\n')
	default:
		return next(stateInWord, b, r)
	}
}

func stateSpaces(b RuneWriter, r rune) (stateFn, int, error) {
	switch r {
	case ' ':
		return stateSpaces, 0, nil
	case '\n':
		return stateBeginLine, 0, nil
	case FuncStart:
		return stateFuncStartSpace, 0, nil
	case FuncEnd:
		return stateSpaces, 0, nil
	case Glue:
		return stateGlueSpace, 0, nil
	case StreamEnd:
		return next(stateBeginText, b, '\n')
	default:
		return next(stateInWord, b, ' ', r)
	}
}

func next(state stateFn, b RuneWriter, runes ...rune) (_ stateFn, n int, err error) {
	n, err = writeRunes(b, runes...)
	return state, n, err
}

func writeRunes(b RuneWriter, runes ...rune) (n int, err error) {
	var m int
	for _, r := range runes {
		m, err = b.WriteRune(r)
		n += m
		if err != nil {
			return
		}
	}
	return
}
