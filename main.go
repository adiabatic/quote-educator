package main

import (
	"bufio"
	"io"
	"log"
	"os"
	"strings"
	"unicode"
)

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
// Blindly copies the interface of io.Copy.
func Educate(out io.Writer, in io.Reader) (written int64, err error) {

	nextDoubleQuoteShouldBeOpening := true
	nextSingleQuoteShouldBeOpening := true

	inBuf := bufio.NewReader(in)
	outBuf := bufio.NewWriter(out)
	defer func() {
		flushErr := outBuf.Flush()
		if flushErr != nil {
			log.Println(err)
		}
	}()

	// defer outBuf.Flush()?

	var r rune
	var N int

	for {
		r = 0
		N = 0

		r, N, err = inBuf.ReadRune()
		if err != nil {
			return written, err
		}
		if r == unicode.ReplacementChar { // U+FFFD
			return written, err
		}

		N = 0

		// NB: this is all kinds of broken and handles lots of things wrongly
		if r == '"' {
			if nextDoubleQuoteShouldBeOpening {
				r = '“'
			} else {
				r = '”'
			}
			nextDoubleQuoteShouldBeOpening = !nextDoubleQuoteShouldBeOpening
		} else if r == '\'' {
			if nextSingleQuoteShouldBeOpening {
				r = '‘'
			} else {
				r = '’'
			}
			nextSingleQuoteShouldBeOpening = !nextSingleQuoteShouldBeOpening
		}

		// end transformation

		N, err = outBuf.WriteRune(r)
		if err != nil {
			return written + int64(N), err
		}
		written += int64(N)
	}
}

func main() {
	Educate(os.Stdout, os.Stdin)
}
