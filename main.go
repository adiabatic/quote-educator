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

// PeekEquals returns true if the reading buffer contains needle starting at the current offset.
func (s *state) PeekEquals(needle string) bool {
	initialOffset := s.currentOffset()

	nb := []byte(needle)
	buf := make([]byte, len(nb))

	if initialOffset <= 0 {
		panic("PeekEquals assumes the first byte has been read, at least")
	}
	_, err := s.r.ReadAt(buf, initialOffset-1)
	if err != nil && err != io.EOF {
		log.Println("Unexpected non-EOF error in PeekEquals")
	}

	return bytes.Equal(nb, buf)
}

// AdvanceUntil reads and writes runes until stopAt is at the current offset.
func (s *state) AdvanceUntil(stopAt string) error {
	for !s.PeekEquals(stopAt) {
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

// func (s *state) SkipUntilString(needle string) error {
// 	nb := []byte(needle)

// 	for {
// 		// buf contains up to and including
// 		buf, err := s.r.ReadBytes(nb[0])
// 		if err != nil {
// 			return err
// 		} // TODO: implement s.ReadBytes and use that instead. It should handle the write counts.

// 	}

// 	return nil
// }

func (s *state) WriteRune(r rune) (size int, err error) {
	return s.w.WriteRune(r)
}

func (s *state) WriteTo(w io.Writer) (n int64, err error) {
	return s.w.WriteTo(w)
}

type callback func(s *state) (next callback, err error)

// TODO: reduce the massive amount of redundant copy/pasted code with inDoubleQuotes
func initial(s *state) (next callback, err error) {
	r, _, err := s.ReadRune()
	if err != nil {
		return nil, err
	}

	next = initial

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

	if s.previous == 'I' { // “I’d”, etc.
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
	if s.currentOffset() == 1 {
		if s.PeekEquals("---") {
			s.WriteRune('-')
			return atYamlFrontMatter, nil
		}
	} else if err != nil {
		return nil, err
	}

	s.WriteRune('-')
	return next, nil
}

func atYamlFrontMatter(s *state) (next callback, err error) {
	next = initial

	const sentinel = "\n---\n"

	err = s.AdvanceUntil(sentinel)
	if err != nil {
		return
	}

	err = s.AdvanceBy(len(sentinel))
	if err != nil {
		return
	}

	return
}

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
