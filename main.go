package main

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"unicode"
)

type state struct {
	// r *bufio.Reader
	//w *bufio.Writer

	r *bytes.Reader
	w bytes.Buffer

	current, previous rune

	readN, writtenN int64 // byte counts
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

func (s *state) WriteTo(w io.Writer) (n int64, err error) {
	return s.w.WriteTo(w)
}

func (s *state) ReadRune() (rune, int, error) {
	r, n, err := s.r.ReadRune()
	if err != nil {
		return r, n, err
	}
	s.readN += int64(n)
	if r == unicode.ReplacementChar { // U+FFFD
		return r, n, errors.New("something got replaced") // TODO: improve this error
	}

	s.previous = s.current
	s.current = r

	return r, n, nil
}

// PeekEquals returns true if needle matches what’s next.
func (s *state) PeekEquals(needle string) bool {
	initialOffset, err := s.r.Seek(0, io.SeekCurrent)
	if err != nil {
		log.Fatalln("Apparently seeking without moving is error-prone:", err)
	}

	nb := []byte(needle)
	buf := make([]byte, len(nb))
	_, err = s.r.ReadAt(buf, initialOffset)
	if err != nil && err != io.EOF {
		log.Println("Unexpected non-EOF error in PeekEquals")
	}

	return bytes.Equal(nb, buf)
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
	size, err = s.w.WriteRune(r)
	if err != nil {
		return
	}
	s.writtenN += int64(size)
	return
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

	// If we're at the beginning of the file then this hyphen could be the start of YAML front matter.

	if offset, err := s.r.Seek(0, io.SeekCurrent); err == nil && offset == 0 {
		if s.PeekEquals("---") {
			return inYamlFrontMatter, nil
		}
	} else if err != nil {
		return nil, err
	}

	s.WriteRune('-')
	return next, nil
}

func inYamlFrontMatter(s *state) (next callback, err error) {
	next = initial

	// err = s.SkipUntilString("\n---\n")

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
