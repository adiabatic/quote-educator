package main

import (
	"bufio"
	"errors"
	"io"
	"log"
	"os"
	"strings"
	"unicode"
)

type state struct {
	r *bufio.Reader
	w *bufio.Writer

	current, previous rune

	readN, writtenN int64 // byte counts
}

func (s *state) ReadRune() (rune, int, error) {
	r, n, err := s.r.ReadRune()
	if err != nil {
		return unicode.ReplacementChar, n, err
	}
	s.readN += int64(n)
	if r == unicode.ReplacementChar { // U+FFFD
		return r, n, errors.New("something got replaced") // TODO: improve this error
	}

	s.previous = s.current
	s.current = r

	return r, n, nil
}

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

	if r == '"' {
		r = '“'
		next = inDoubleQuotes
	} else if r == '\'' {
		// don’t assign r — we’re not sure if it’s going to be an opening single quote or an apostrophe
		return atSingleQuote, nil
	}

	_, err = s.WriteRune(r)
	if err != nil {
		return nil, err
	}
	return next, nil
}

func inDoubleQuotes(s *state) (next callback, err error) {
	r, _, err := s.ReadRune()
	if err != nil {
		return nil, err
	}

	next = inDoubleQuotes

	if r == '"' {
		r = '”'
		next = initial
	}

	_, err = s.WriteRune(r)
	if err != nil {
		return nil, err
	}
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

	_, err = s.WriteRune(r)
	if err != nil {
		return nil, err
	}
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

	_, err = s.WriteRune(r)
	if err != nil {
		return nil, err
	}
	return next, nil
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

	inBuf := bufio.NewReader(in)
	outBuf := bufio.NewWriter(out)
	defer func() {
		flushErr := outBuf.Flush()
		if flushErr != nil {
			log.Println(err)
		}
	}()

	var s state
	s.r = inBuf
	s.w = outBuf
	// leave readN and writtenN at 0 each for obvious reasons

	f := initial

	for {
		f, err = f(&s)
		if err != nil {
			return s.writtenN, err
		}
	}
}

func main() {
	Educate(os.Stdout, os.Stdin)
}
