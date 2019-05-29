package main

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"unicode"
)

// A state struct contains information that the parser needs to keep track of.
//
// Ordinarily I’d call this type a “parser”, but all the what-to-do-when functions are as much of the parser as this bag of state is.
type state struct {
	r *bytes.Reader
	w bytes.Buffer

	current, previous rune
}

func newState(whence io.Reader) (state, error) {
	var s state

	whenceContents, err := ioutil.ReadAll(whence)
	if err != nil {
		return s, err
	}

	s.r = bytes.NewReader(whenceContents)

	return s, nil
}

func (s *state) ReadRune() (rune, int, error) {
	r, n, err := s.r.ReadRune()
	if err != nil {
		return r, n, err // …without updating
	}

	s.previous = s.current
	s.current = r

	return r, n, nil
}

func (s *state) currentOffset() int64 {
	i, err := s.r.Seek(0, io.SeekCurrent)
	if err != nil {
		panic(err) // I looked at the bytes.Reader source as of 2019-05-21 and this should never happen
	}
	return i
}

// PeekEquals returns true if needle is just ahead of the current offset.
//
// The next call to ReadRune will return the first character of needle.
func (s *state) PeekEquals(needle string) bool {
	nb := []byte(needle)
	buf := make([]byte, len(nb))

	_, err := s.r.ReadAt(buf, s.currentOffset())
	if err != nil && err != io.EOF {
		log.Println("Unexpected non-EOF error in PeekEquals")
	}

	return bytes.Equal(nb, buf)
}

// AdvanceUntil reads and writes runes until stopBefore is just ahead of the current offset.
//
// In other words, once AdvanceUntil returns with a non-nil error, the next rune read will match the start of stopBefore.
func (s *state) AdvanceUntil(stopBefore string) error {
	for !s.PeekEquals(stopBefore) {
		r, _, err := s.ReadRune()
		if err != nil {
			return err
		}
		s.WriteRune(r)
	}

	return nil
}

// AdvanceBy reads and writes n runes.
func (s *state) AdvanceBy(n int) error {
	for ; n > 0; n-- {
		r, _, err := s.ReadRune()
		if err != nil {
			return err
		}
		s.WriteRune(r)
	}

	return nil
}

func (s *state) AdvanceThrough(stopAfter string) error {
	if err := s.AdvanceUntil(stopAfter); err != nil {
		return err
	}

	if err := s.AdvanceBy(len(stopAfter)); err != nil {
		return err
	}

	return nil
}

func (s *state) WriteRune(r rune) (size int, err error) {
	return s.w.WriteRune(r)
}

func (s *state) WriteTo(w io.Writer) (n int64, err error) {
	return s.w.WriteTo(w)
}

// A stateFunction specifies what to do to keep parsing the input given what’s come before.
//
// Conventions:
//
// Here’s the situation for the insides of stateFunctions that start with “at” (like “atHyphen” or “atYAMLFrontMatter”):
//
// - the current rune (i.e. the one that would be returned by state.ReadRune()) should be whatever comes just after the first rune in a series (like either a standalone hyphen or the first hyphen of three that starts a YAML front-matter block)
// - they don’t call s.ReadRune() themselves (use state.PeekEquals() to see what’s next)
//
// Similarly, here’s the situation for the insides of stateFunctions that start with “in” (like “inSingleQuotes”):
//
// - the rune that would be returned by state.ReadRune() is something that’s inside the function’s namesake (like “f” or “a” or maybe the final “`” of “`fread()`”)
// - they call s.ReadRune() pretty much at the top of the function
//
// Incidentally, http://journal.stuffwithstuff.com/2011/03/19/pratt-parsers-expression-parsing-made-easy/ calls this type a “parselet”. Maybe that’d be a better name.
type callback func(s *state) (next callback, err error)

// These functions are sorted by character. That is, atYamlFrontMatter (starts with ---) should come shortly after atHyphen (-).

func initial(s *state) (next callback, err error) {
	r, _, err := s.ReadRune()
	if err != nil {
		return nil, err
	}

	next = initial

	// The style for now:
	// - lexing (differentiating between hyphens and a YAML Front Matter block) is fused with parsing
	// - in* get the runes written immediately
	// - at* get the runes written at the earliest possible at* (atHyphen, not atYAMLFrontMatter)
	switch r {
	case '"', '“':
		r = '“'
		next = inDoubleQuotes
	case '\'':
		// don’t assign r — we’re not sure if it’s going to be an opening single quote or an apostrophe
		return atSingleQuote, nil
	case '-':
		// could be a YAML front matter or all sorts of fancy things
		return atHyphen, nil
	case '`':
		return atBacktick, nil
	}

	s.WriteRune(r)
	return next, nil
}

func inDoubleQuotes(s *state) (next callback, err error) {
	r, _, err := s.ReadRune()
	if err != nil {
		return nil, err
	}

	next = inDoubleQuotes

	switch r {
	case '"', '”':
		r = '”'
		next = initial
	}

	s.WriteRune(r)
	return next, nil
}

func atSingleQuote(s *state) (next callback, err error) {
	r := unicode.ReplacementChar // Don’t read anything yet

	next = initial

	if unicode.IsLetter(s.previous) { // “I’d”, etc.
		r = '’'
	} else {
		r = '‘'
		next = inSingleQuotes
	}

	s.WriteRune(r)
	return next, nil
}

func inSingleQuotes(s *state) (next callback, err error) {
	r, _, err := s.ReadRune()
	if err != nil {
		return nil, err
	}

	next = inSingleQuotes

	if r == '\'' {
		r = '’'
		next = initial
	}

	s.WriteRune(r)
	return next, nil
}

func atHyphen(s *state) (next callback, err error) {
	next = initial

	// If we’ve read only a hyphen at offset 0 and are about to read a character at offset 1, then this might start a YAML front-matter block
	if s.currentOffset() == 1 && s.PeekEquals("--") {
		next = atYamlFrontMatter
	}

	s.WriteRune('-')
	return next, nil
}

func atYamlFrontMatter(s *state) (next callback, err error) {
	next = initial

	err = s.AdvanceThrough("\n---\n")
	// non-nil error or not, it's getting returned anyway

	return
}

func atBacktick(s *state) (next callback, err error) {
	next = atCodeSpan

	if s.PeekEquals("``") {
		next = atBacktickFence
	}

	s.WriteRune('`')
	return
}

func atCodeSpan(s *state) (next callback, err error) {

	return
}

func atBacktickFence(s *state) (next callback, err error) {
	next = initial

	err = s.AdvanceThrough("\n```\n")
	// non-nil or not, err gets returned

	return
}

// Not yet added: in/at functions for: \, <, HTML element names, HTML element attributes, HTML element attribute values, old-school four-indent preformatted-code blocks
// Open question: Does the parser, inside a callback function, always know what should come next? Or can a thingo that a callback function is handling show up in multiple contexts? Granted, one could hack around this with functions named "fooInABaz" vs. "fooInAQuux"…and that might be better than maintaining a stack.

// EducateString is a convenience function for running Educate on strings.
func EducateString(s string) (string, error) {
	sr := strings.NewReader(s)
	out := &strings.Builder{}

	_, err := Educate(out, sr)
	if err != nil && err != io.EOF {
		return "", err
	}

	return out.String(), nil
}

// Educate curls quotes from in and writes them to out.
//
// Blindly copies the interface of io.Copy without deeply considering why it has the return values it has.
func Educate(out io.Writer, in io.Reader) (written int64, err error) {
	s, err := newState(in)
	if err != nil {
		return 0, err
	}

	f := initial

	for {
		f, err = f(&s)
		if err != nil { // probably just an EOF
			break
		}
	}

	if err != nil && err != io.EOF {
		return 0, err
	}

	return s.WriteTo(out)
}

func main() {
	N, err := Educate(os.Stdout, os.Stdin)
	if err != nil {
		log.Printf("%v bytes written before an error occurred: %v", N, err)
		os.Exit(1)
	}
}
