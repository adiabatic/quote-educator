package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"unicode"
	"unicode/utf8"
)

// A state struct contains information that the parser needs to keep track of.
//
// Ordinarily I’d call this type a “parser”, but all the what-to-do-when functions are as much of the parser as this bag of state is.
type state struct {
	r *bytes.Reader
	w bytes.Buffer

	whatDo map[rune]callback

	codeElementsEntered int
}

func newState(whence *bytes.Reader) (state, error) {
	var s state

	if whence == nil {
		return s, errors.New("newState: nil *bytes.Reader to read from")
	}

	s.r = whence

	s.whatDo = make(map[rune]callback)

	s.whatDo['\\'] = atBackslash

	s.whatDo['"'] = atDoubleQuote
	s.whatDo['“'] = atDoubleQuote

	s.whatDo['\''] = atSingleQuote
	s.whatDo['‘'] = atSingleQuote

	s.whatDo['-'] = atHyphen

	s.whatDo['`'] = atBacktick

	s.whatDo['<'] = atLessThan

	return s, nil
}

func (s *state) readRune() (rune, error) {
	r, _, err := s.r.ReadRune()
	if err != nil {
		return r, err // …without updating
	}

	return r, nil
}

func (s *state) previousRune() rune {
	r, size := utf8.DecodeLastRune(s.w.Bytes())
	if size == 0 {
		panic("Couldn’t decode the last rune in s.w.Bytes()")
	}
	return r
}

func (s *state) mustReadRune() rune {
	r, err := s.readRune()
	if err != nil {
		panic(err)
	}
	return r
}

func (s *state) peekRune() (rune, error) {
	r, _, err := s.r.ReadRune()
	if err != nil {
		return r, err
	}

	err = s.r.UnreadRune()
	return r, err
}

// mustPeekRune is only for debug code.
func (s *state) mustPeekRune() rune {
	r, err := s.peekRune()
	if err != nil {
		panic(err)
	}

	return r
}

func (s *state) unreadRune() error {
	return s.r.UnreadRune()
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

// AdvanceBy reads and writes n runes.
func (s *state) AdvanceBy(n int) error {
	for ; n > 0; n-- {
		r, err := s.readRune()
		if err != nil {
			return err
		}
		s.writeRune(r)
	}

	return nil
}

// A runePredicate returns true when the given rune exhibits some property.
type runePredicate func(rune) bool

// AdvanceUntil reads and writes runes until stopBefore is just ahead of the current offset.
//
// In other words, once AdvanceUntil returns with a non-nil error, the next rune read will match the start of stopBefore.
func (s *state) AdvanceUntil(stopBefore string) error {
	for !s.PeekEquals(stopBefore) {
		r, err := s.readRune()
		if err != nil {
			return err
		}
		s.writeRune(r)
	}

	return nil
}

// AdvanceUntilTrue reads and writes from s, stopping only when the peeked-at rune matches a predicate.
func (s *state) AdvanceUntilTrue(f runePredicate) error {
	for {
		p, err := s.peekRune()
		if err != nil {
			return err
		}

		if f(p) {
			return nil
		}
		s.writeRune(s.mustReadRune())
	}
}

// AdvanceUntilFalse reads and writes from s, stopping only when the peeked-at rune doesn’t a predicate.
func (s *state) AdvanceUntilFalse(f runePredicate) error {
	g := func(r rune) bool { return !f(r) }
	return s.AdvanceUntilTrue(g)
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

// Reads and writes one rune if the passed-through error is nil.
func (s *state) advanceOneMore(err error) error {
	if err != nil {
		return err
	}

	r, err := s.readRune()
	if err != nil {
		return err
	}

	return s.writeRune(r)
}

func (s *state) writeRune(r rune) error {
	_, err := s.w.WriteRune(r)
	return err
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
type callback func(s *state) error

// These functions are sorted by character. That is, atYAMLFrontMatter (starts with ---) should come shortly after atHyphen (-).

func initial(s *state) error {
	var r rune
	var err error
	for err == nil {
		r, err = s.peekRune()
		if err != nil {
			return err
		}

		if f, ok := s.whatDo[r]; ok {
			err = f(s)
		} else {
			s.writeRune(s.mustReadRune())
		}
	}

	return err
}

func atBackslash(s *state) error {
	r := s.mustReadRune()
	if r != '\\' {
		return fmt.Errorf("expected read rune to be \\ in atBackslash. got: «%s» (%U)", string(r), r)
	}

	s.writeRune(r)
	r, err := s.readRune()
	if err != nil {
		return err
	}
	s.writeRune(r)
	return err
}

func atDoubleQuote(s *state) error {
	r := s.mustReadRune()
	if !(r == '"' || r == '“') {
		return fmt.Errorf("expected read rune to be \" or “ in atDoubleQuote. got: «%s» (%U)", string(r), r)
	}

	s.writeRune('“')
	return inDoubleQuotes(s)
}

func inDoubleQuotes(s *state) error {
	var r rune
	var err error
	for err == nil {
		r, err = s.readRune()
		if err != nil {
			break
		}

		if r == '"' || r == '”' {
			return s.writeRune('”')
		} else if f, ok := s.whatDo[r]; ok {
			s.unreadRune()
			err = f(s)
		} else {
			s.writeRune(r)
		}
	}

	return err
}

func atSingleQuote(s *state) error {
	r := s.mustReadRune()
	if !(r == '\'' || r == '‘') {
		return fmt.Errorf("Expecting a single quote, either curly or straight. got: «%s» (%U)", string(r), r)
	}

	if unicode.IsLetter(s.previousRune()) {
		return s.writeRune('’')
	}

	s.writeRune('‘')
	return inSingleQuotes(s)
}

func inSingleQuotes(s *state) error {
	var r rune
	var err error
	for err == nil {
		r, err = s.readRune()
		if err != nil {
			break
		}

		if r == '\'' || r == '’' {
			return s.writeRune('’')
		} else if f, ok := s.whatDo[r]; ok {
			err = f(s)
		} else {
			s.writeRune(r)
		}
	}
	return err
}

func atHyphen(s *state) error {
	r := s.mustReadRune()
	if r != '-' {
		return fmt.Errorf("Expecting a hyphen. got: «%s» (%U)", string(r), r)
	}

	if s.currentOffset() == 1 && s.PeekEquals("--") {
		s.writeRune(r)
		return inYAMLFrontMatter(s)
	}

	return s.writeRune(r)
}

func inYAMLFrontMatter(s *state) error {
	return s.AdvanceThrough("\n---\n") // Just don’t do anything
}

func atBacktick(s *state) error {
	r := s.mustReadRune()
	if r != '`' {
		return fmt.Errorf("expecting a backtick. got: «%s» (%U)", string(r), r)
	}

	if s.PeekEquals("``") && s.previousRune() == '\n' {
		s.writeRune(r)
		return inTripleBacktickCodeBlock(s)
	}

	s.writeRune(r)
	return inSingleBacktickCodeSpan(s)
}

func inSingleBacktickCodeSpan(s *state) error {
	for {
		r, err := s.readRune()
		if err != nil {
			return err
		}

		previousRune := s.previousRune()

		s.writeRune(r) // after this call, r would be returned by s.previousRune()

		if r == '`' && previousRune != '\\' {
			break
		}
	}

	return nil
}

func inTripleBacktickCodeBlock(s *state) error {
	return s.AdvanceThrough("\n```\n") // Just don’t do anything here, either
}

func atLessThan(s *state) error {
	r := s.mustReadRune()
	if r != '<' {
		return fmt.Errorf("expecting a less-than symbol (<). got: «%s» (%U)", string(r), r)
	}

	s.writeRune(r)

	r, err := s.readRune()
	if err != nil {
		return err
	}

	// https://html.spec.whatwg.org/multipage/syntax.html#syntax-tag-name notwithstanding, no elements are ever going to *start* with a *number*, right?
	if unicode.IsLetter(r) {
		s.writeRune(r)
		return inHTMLStartTagName(s)
	}

	if r == '/' {
		s.writeRune(r)
		return inHTMLEndTagName(s)
	}
	return s.writeRune(r)
}

func inHTMLStartTagName(s *state) error {
	// The first (and possibly only) letter of the element name has already been written.

	var p rune
	//var r rune
	var err error

	// Are we entering a code element? They’re special because we don’t curl quotes there.
	codeElementsEnteredAtStart := s.codeElementsEntered
	if s.previousRune() == 'c' && s.PeekEquals("ode") {
		s.codeElementsEntered++
	}

	// Read and write the element name.
	// When this is done, the last letter of the element name will be freshly written.
	// The next rune will be either whitespace (maybe junk, maybe preceding an attribute) or >.
	for {
		p, err = s.peekRune()
		if err != nil {
			log.Println("Unexpected error peeking rune in inHTMLStartTagName")
			return err
		}

		if isASCIIWhitespace(p) || p == '>' {
			log.Printf("At the break point, p was: %s (%U)", string(p), p)
			break
		}

		s.writeRune(s.mustReadRune())
	}

	p, err = s.peekRune()
	if err != nil {
		return err
	}
	if !(p == '>' || isASCIIWhitespace(p)) {
		log.Fatalf("postcondition failed. was expecting p to be either > or whitespace; was %s (%U)", string(p), p)
	}

	// Now we need to advance past any whitespace so s.peekRune() gives us either an attribute name or >.
	{
		err = s.advanceOneMore(s.AdvanceUntilFalse(isASCIIWhitespace))
		if err != nil {
			return err
		}
	}

	log.Printf("After advancing through the whitespace, op is %s%s",
		string(s.previousRune()), string(s.mustPeekRune()))

	if s.previousRune() == '>' {
		if s.codeElementsEntered > codeElementsEnteredAtStart {
			return inCodeElement(s)
		}
	} else if unicode.IsLetter(s.previousRune()) {
		err = handleHTMLAttributes(s)
		if err != nil {
			return err
		}
	}

	// TODO: Handle attributes (that is, don’t curl attribute quotes)
	return err
}

// handleHTMLAttributes does the obvious. When it ends, s.peekRune() will return >.
func handleHTMLAttributes(s *state) error {
	// s.previousRune() is the first letter of the attribute name
	err := s.AdvanceUntilFalse(isLegalHTMLAttributeNameRune)
	if err != nil {
		return err
	}

	p, err := s.peekRune()
	if err != nil {
		return err
	}

	// Move past the whitespace until we get to what should be either a > or =.
	if isASCIIWhitespace(p) {
		err = s.AdvanceUntilFalse(isASCIIWhitespace)
		if err != nil {
			return err
		}
	}

	// check postcondition
	p, err = s.peekRune()
	if err != nil {
		return err
	}
	if !(p == '>' || p == '=') {
		log.Fatalf("postcondition failed. p was expected to be either > or =, but was %s instead", string(p))
	}

	log.Println(string(s.mustPeekRune()))

	if p == '>' {
		return nil
	}

	// p has to be =, then.

	s.writeRune(s.mustReadRune())

	p, err = s.peekRune()
	if err != nil {
		return err
	}

	// Move past the whitespace until we get to a ", ', or the characters of an unquoted attribute value.
	// The full rules: https://html.spec.whatwg.org/multipage/syntax.html#syntax-attributes
	if isASCIIWhitespace(p) {
		err = s.AdvanceUntilFalse(isASCIIWhitespace)
		if err != nil {
			return err
		}
	}

	// What kind of attribute value do we have?

	p, err = s.peekRune()
	if err != nil {
		return err
	}

	switch {
	case p == '"':
		s.writeRune(s.mustReadRune())
		err = inDoubleQuotedAttributeValue(s)
		// case unicode.IsLetter(p), unicode.IsNumber(p), p == '/':
		// default:
	}

	if err != nil {
		return err
	}

	return err
}

func inDoubleQuotedAttributeValue(s *state) error {
	// yes, this is totally copied from inSingleBacktickCodeSpan where I changed only one character
	for {
		r, err := s.readRune()
		if err != nil {
			return err
		}

		previousRune := s.previousRune()

		s.writeRune(r) // after this call, r would be returned by s.previousRune()

		if r == '"' && previousRune != '\\' {
			break
		}
	}

	return nil
}

func inHTMLEndTagName(s *state) error {
	if s.PeekEquals("code") {
		s.codeElementsEntered--
	}

	return s.AdvanceThrough(">")
}

func inCodeElement(s *state) error {
	err := s.AdvanceThrough("</code")
	if err != nil {
		return err
	}

	err = s.AdvanceUntilTrue(isASCIIWhitespace)
	if err != nil {
		return err
	}

	return s.writeRune(s.mustReadRune())
}

// Not yet added: in/at functions for: <, HTML element names, HTML element attributes, HTML element attribute values, old-school four-indent preformatted-code blocks

// Educate curls quotes from in and writes them to out.
//
// Blindly copies the interface of io.Copy without deeply considering why it has the return values it has.
func Educate(out io.Writer, in *bytes.Reader) (written int64, err error) {
	s, err := newState(in)
	if err != nil {
		return 0, err
	}

	err = initial(&s)

	if err != nil && err != io.EOF {
		return 0, err
	}

	return s.WriteTo(out)
}

func main() {
	var whence io.Reader = os.Stdin
	var whither = os.Stdout

	rewriteInPlace := flag.Bool("w", false, "write result to (source) file instead of stdout")
	flag.Parse()
	continueRewriteThings := false

	if rewriteInPlace != nil && *rewriteInPlace {
		switch len(flag.Args()) {
		case 0:
			log.Println("Must specify a file to overwrite with -w")
			os.Exit(2)
		case 1:
			// continue
		default:
			log.Println("Must specify only one file to overwrite with -w")
			os.Exit(3)
		}

		continueRewriteThings = true
		var err error
		whence, err = os.Open(flag.Args()[0])
		if err != nil {
			log.Printf("Could not open file named “%s” for both reading and writing: %v\n", flag.Args()[0], err)
			os.Exit(4)
		}
	}

	whenceContents, err := ioutil.ReadAll(whence)
	if err != nil {
		log.Println("Something went wrong when reading input: ", err)
	}

	whenceReader := bytes.NewReader(whenceContents)

	if continueRewriteThings {
		// now that we’ve got the input all slurped up, let’s set up the out piping

		whither, err = os.OpenFile(flag.Args()[0], os.O_WRONLY|os.O_TRUNC, 0755) // BUG(adiabatic): cargo-culting the “0755”; I don’t understand masks

	}

	N, err := Educate(whither, whenceReader)
	if err != nil {
		log.Printf("%v bytes written before an error occurred: %v", N, err)
		os.Exit(1)
	}
	err = whither.Sync()
	if err != nil {
		log.Printf("couldn’t flush stdout: %v", err)
		os.Exit(2)
	}
}

func isASCIIWhitespace(r rune) bool {
	switch r {
	case 0x0009, 0x000a, 0x000c, 0x000d, 0x0020: // tab, linefeed, form feed, carriage return, space
		return true
	}
	return false
}

func isLegalHTMLAttributeNameRune(r rune) bool {
	// https://html.spec.whatwg.org/multipage/syntax.html#syntax-attributes
	if unicode.IsControl(r) { // should include tab
		return false
	}

	switch r {
	case ' ', '"', '\'', '>', '/', '=':
		return false
	}

	// Full list of noncharacters: https://infra.spec.whatwg.org/#noncharacter
	table := unicode.RangeTable{
		R16: []unicode.Range16{{0xfdd0, 0xfdef, 1}, {0xfffe, 0xffff, 1}},
		// BUG(adiabatic): Erroneously thinks non-BMP noncharacters are characters
	}
	if unicode.In(r, &table) {
		return false
	}

	return true
}
