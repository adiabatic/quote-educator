// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
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

func (s *state) mustPreviousRune() rune {
	r, size := utf8.DecodeLastRune(s.w.Bytes())
	if size == 0 {
		panic("Couldn’t decode the last rune in s.w.Bytes()")
	}
	return r
}

func (s *state) previousRune() (rune, error) {
	r, size := utf8.DecodeLastRune(s.w.Bytes())
	if size == 0 {
		return 0, errors.New("BOF")
	}
	return r, nil
}

func (s *state) previousRuneMatches(f func(rune) bool) bool {
	r, err := s.previousRune()
	if err != nil {
		return false
	}
	return f(r)
}

func (s *state) previousRuneMatchesAny(candidates ...rune) bool {
	for _, candidate := range candidates {
		if r, err := s.previousRune(); err != nil {
			if r == candidate {
				return true
			}
		}
	}
	return false
}

func (s *state) previousRunesMatchOne(candidate string) bool {
	bs := s.w.Bytes()
	n := utf8.RuneCountInString(candidate)
	puddle := make([]rune, n)

	for ; n > 0; n-- {
		r, size := utf8.DecodeLastRune(bs)
		if size == 0 {
			return false
		}

		puddle[n-1] = r
		bs = bs[:len(bs)-size]
	}

	return string(puddle) == candidate
}

func (s *state) previousRunesMatchAny(candidates ...string) (needle string, ok bool) {
	for _, candidate := range candidates {
		if s.previousRunesMatchOne(candidate) {
			return candidate, true
		}
	}

	return "", false
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

// AdvanceUntil reads and writes runes until the next rune to be read is the first rune in stopBefore.
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

// AdvanceUntilTrue reads and writes from s. When the peeked-at rune doesn’t match the given predicate, it stops. The peeked-at rune remains unread.
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

// AdvanceUntilFalse reads and writes from s. When the peeked-at rune matches the given predicate, it stops. The peeked-at rune remains unread.
func (s *state) AdvanceUntilFalse(f runePredicate) error {
	g := func(r rune) bool { return !f(r) }
	return s.AdvanceUntilTrue(g)
}

// AdvanceThrough reads and writes runes until the next rune to be read is the one right after stopAfter.
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

// initial contains the main loop of the parser. It peeks at the next rune and checks to see if it gets special processing according to the whatDo map. If special processing may be called for, a special-processing function will be called. Otherwise, it just reads and writes the peeked-at rune.
//
// Ends and returns if an error is encountered, although that may just be an io.EOF.
func initial(s *state) error {
	var p rune
	var err error
	for err == nil {
		p, err = s.peekRune()
		if err != nil {
			return err
		}

		if f, ok := s.whatDo[p]; ok {
			err = f(s)
		} else {
			s.writeRune(s.mustReadRune())
		}
	}

	return err
}

// atBackslash reads an assumed-to-exist \ and writes both it and the rune after it without further processing or examination.
//
// When atBackslash returns, readRune will return the rune after the rune after the backslash.
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

// atDoubleQuote reads an assumed-to-exist " or “. It then writes a “ and hands processing off to inDoubleQuotes.
func atDoubleQuote(s *state) error {
	r := s.mustReadRune()
	if !(r == '"' || r == '“') {
		return fmt.Errorf("expected read rune to be \" or “ in atDoubleQuote. got: «%s» (%U)", string(r), r)
	}

	s.writeRune('“')
	return inDoubleQuotes(s)
}

// inDoubleQuotes reads and writes runes inside double quotes, looking for some sort of closing double quote (either " or ”).
//
// Ends and returns if a closing double quote is read or an error is encountered, although that may just be an io.EOF. The next rune to be read will be the one just after the closing double quote.
func inDoubleQuotes(s *state) error {
	var p rune
	var err error
	for err == nil {
		p, err = s.peekRune()
		if err != nil {
			break
		}

		if p == '"' || p == '”' {
			// normally we immediately write the freshly-read previously-peeked-at rune, but we want a ” in the output whether the input had a "or ”, so we just drop the maybe-educated freshly-read previously-peeked-at quote-mark rune on the floor
			_ = s.mustReadRune()
			return s.writeRune('”')
		} else if f, ok := s.whatDo[p]; ok {
			err = f(s)
		} else {
			s.writeRune(s.mustReadRune())
		}
	}

	return err
}

// atSingleQuote reads an assumed-to-exist ' or ‘ rune. It then writes a ‘ or ’ depending on whether the previous rune was a letter or not, as a ' right after a letter is probably being used as an apostrophe.
//
// TODO(adiabatic): Doesn’t do the right thing for cases like <a>Mark Twain</a>'s autobiography.
// BUG(adiabatic): This function will go to the inSingleQuotes state if the rune was ‘ and was preceded by a letter. Could be bad for Arabic in romanization, Hawaiian, and Maori (among others).
func atSingleQuote(s *state) error {
	r := s.mustReadRune()
	if !(r == '\'' || r == '‘') {
		return fmt.Errorf("expecting a single quote, either curly or straight. got: «%s» (%U)", string(r), r)
	}

	if s.previousRuneMatches(unicode.IsLetter) {
		return s.writeRune('’')
	}

	if r == '\'' && s.previousRuneMatchesAny('>', ')') {
		log.Printf("Found the string «%s'»; cannot tell whether this is a quote mark or an apostrophe. Leaving unchanged. Manually inspect subsequent quote marks.", string(s.mustPreviousRune()))
		return s.writeRune('\'')
	}

	s.writeRune('‘')
	return inSingleQuotes(s)
}

// inSingleQuotes reads and writes runes inside single quotes, looking for some sort of closing single quote (either ' or ’).
//
// Ends and returns if a closing single quote is read or an error is encountered, although that may just be an io.EOF. The next rune to be read will be the one just after the closing single quote.
//
// BUG(adiabatic): will mistakenly identify «don't» as a closing single quote
func inSingleQuotes(s *state) error {
	var p rune
	var err error
	for err == nil {
		p, err = s.peekRune()
		if err != nil {
			break
		}

		if p == '\'' || p == '’' {
			// deliberately drop it on the floor (see comment in inDoubleQuotes)
			_ = s.mustReadRune()

			if needle, ok := s.previousRunesMatchAny("can", "you", "don"); ok {
				// this was probably an apostrophe in a contraction
				log.Printf("The string «%s» was found right before an apostrophe inside of a single-quote quote. The apostrophe was assumed to be part of a contraction. Double-check the output to verify this was the case.", needle)
				s.writeRune('’')
				continue
			}
			return s.writeRune('’')

		} else if f, ok := s.whatDo[p]; ok {
			err = f(s)
		} else {
			s.writeRune(s.mustReadRune())
		}
	}
	return err
}

// atHyphen reads an assumed-to-exist - and checks to see if it could be the start of YAML front matter or not.
//
// When it returns, if the rune was just a hyphen, the next character to be read will be the character after the hyphen. However, if the hyphen was the first of a YAML front matter block, the next character to be read will be whatever inYAMLFrontMatter says it will be.
func atHyphen(s *state) error {
	r := s.mustReadRune()
	if r != '-' {
		return fmt.Errorf("expecting a hyphen. got: «%s» (%U)", string(r), r)
	}

	if s.currentOffset() == 1 && s.PeekEquals("--") {
		s.writeRune(r)
		return inYAMLFrontMatter(s)
	}

	return s.writeRune(r)
}

// inYAMLFrontMatter just reads and writes until it gets past a --- all on its own line.
//
// When inYAMLFrontMatter returns, the next rune to be read will be the first rune on the line after the closing ---.
func inYAMLFrontMatter(s *state) error {
	return s.AdvanceThrough("\n---\n") // Just don’t do anything
}

// atBacktick reads an assumed-to-exist `. It then peeks ahead and behind to figure out whether this is the start of a single-backtick code span or a triple-backtick code block.
func atBacktick(s *state) error {
	r := s.mustReadRune()
	if r != '`' {
		return fmt.Errorf("expecting a backtick. got: «%s» (%U)", string(r), r)
	}

	if s.PeekEquals("``") && s.previousRuneMatchesAny('\n') {
		s.writeRune(r)
		return inTripleBacktickCodeBlock(s)
	}

	s.writeRune(r)
	return inSingleBacktickCodeSpan(s)
}

// inSingleBacktickCodeSpan reads and writes runes inside a single-backtick code span. When it returns, the next rune to be read will be the one after the closing backtick.
func inSingleBacktickCodeSpan(s *state) error {
	return inSpanEndingWithSingleUnescapedRune(s, '`')
}

// inSpanEndingWithSingleUnescapedRune reads and writes runes until it gets to a sentinel character not preceded by a backslash. When it returns, the next rune to be read will be the one after the sentinel value.
func inSpanEndingWithSingleUnescapedRune(s *state, sentinel rune) error {
	for {
		r, err := s.readRune()
		if err != nil {
			return err
		}

		previousRune := s.mustPreviousRune()

		s.writeRune(r) // after this call, r would be returned by s.previousRune()

		if r == sentinel && previousRune != '\\' {
			break
		}
	}

	if v := s.mustPreviousRune(); v != sentinel {
		return fmt.Errorf("postcondition failed: expected the immediately previous rune to be a %s. got: «%s» (%U)", string(sentinel), string(v), v)
	}

	return nil
}

// inTripleBacktickCodeBlock just reads and writes until it gets past a ``` all on its own line.
//
// When inTripleBacktickCodeBlock returns, the next rune to be read will be the first rune on the line after the closing ```.
func inTripleBacktickCodeBlock(s *state) error {
	return s.AdvanceThrough("\n```\n") // Just don’t do anything here, either
}

// atLessThan reads an assumed-to-exist <. It then peeks ahead to figure out whether the < is a mere less-than sign or the start of an HTML tag.
//
// When atLessThan returns, readRune will return the rune right after the < (or an error).
func atLessThan(s *state) error {
	r := s.mustReadRune()
	if r != '<' {
		return fmt.Errorf("expecting a less-than symbol (<). got: «%s» (%U)", string(r), r)
	}

	s.writeRune(r)

	p, err := s.peekRune()
	if err != nil {
		return err
	}

	// https://html.spec.whatwg.org/multipage/syntax.html#syntax-tag-name notwithstanding, no elements are ever going to *start* with a *number*, right?
	if unicode.IsLetter(p) {
		return inHTMLStartTagName(s)
	}

	if p == '/' {
		return inHTMLEndTagName(s)
	}

	return s.writeRune(s.mustReadRune())
}

// inHTMLStartTagName reads and writes an HTML start tag.
//
// When it finishes, the current rune is either
// the rune right after the tag’s closing >
// or the first character of the first attribute’s name.
func inHTMLStartTagName(s *state) error {
	var p rune
	var err error

	// Are we entering a code element? They’re special because we don’t curl quotes there.
	codeElementsEnteredAtStart := s.codeElementsEntered
	if s.PeekEquals("code") {
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
			// log.Printf("The first not-a-start-tag rune was «%s» (%U)", string(p), p)
			break
		}

		s.writeRune(s.mustReadRune())
	}

	if p = s.mustPeekRune(); !(p == '>' || isASCIIWhitespace(p)) {
		log.Fatalf("postcondition failed. was expecting p to be either > or whitespace; was «%s» (%U)", string(p), p)
	}

	// Now we need to advance past any whitespace so s.peekRune() gives us either an attribute name or >.
	err = s.AdvanceUntilFalse(isASCIIWhitespace)
	if err != nil {
		return err
	}

	p = s.mustPeekRune()
	if p == '>' {
		if s.codeElementsEntered > codeElementsEnteredAtStart {
			return inCodeElement(s)
		}
	} else if unicode.IsLetter(p) {
		err = handleHTMLAttributes(s)
		if err != nil {
			return err
		}
		p, err = s.peekRune()
		if err != nil {
			return err
		}
		if s.codeElementsEntered > codeElementsEnteredAtStart {
			return inCodeElement(s)
		} // no special handling for non-code HTML attributes
	}

	return err
}

// handleHTMLAttributes churns through HTML attributes. When it ends, s.peekRune() will return >.
func handleHTMLAttributes(s *state) error {
	var p rune
	var err error

	for err == nil {

		p = s.mustPeekRune()

		if isASCIIWhitespace(p) {
			err = s.AdvanceUntilFalse(isASCIIWhitespace)
			if err != nil {
				return err
			}
		}

		// log.Printf("The first character of the HTML attribute name is: «%s» (%U)", string(p), p)

		// Churn through the attribute name.
		err = s.AdvanceUntilFalse(isLegalHTMLAttributeNameRune)
		if err != nil {
			return err
		}

		// Churn through any whitespace until we get to what should be either a > or =.
		if isASCIIWhitespace(s.mustPeekRune()) {
			err = s.AdvanceUntilFalse(isASCIIWhitespace)
			if err != nil {
				return err
			}
		}

		if p = s.mustPeekRune(); !(p == '>' || p == '=') {
			log.Fatalf("postcondition failed. p was expected to be either > or =, but was «%s» instead", string(p))
		}

		if p == '>' {
			// no more attributes to handle
			return nil
		}

		// p has to be =, then. Pump it.
		s.writeRune(s.mustReadRune())

		p, err = s.peekRune()
		if err != nil {
			return err
		}

		// Move past any existing whitespace until we get to a ", ', or the characters of an unquoted attribute value.
		// The full rules: https://html.spec.whatwg.org/multipage/syntax.html#syntax-attributes
		if isASCIIWhitespace(p) {
			err = s.AdvanceUntilFalse(isASCIIWhitespace)
			if err != nil {
				return err
			}
		}

		// What kind of attribute value do we have? Unquoted, single-quoted, or double-quoted?

		p, err = s.peekRune()
		if err != nil {
			return err
		}

		switch {
		case p == '"':
			s.writeRune(s.mustReadRune())
			err = inDoubleQuotedAttributeValue(s)
		case p == '\'':
			s.writeRune(s.mustReadRune())
			err = inSingleQuotedAttributeValue(s)
		case isLegalHTMLAttributeValueUnquoted(p):
			err = inUnquotedAttributeValue(s)
		default:
			err = fmt.Errorf("Got some weird rune that’s starting an HTML attribute value: «%s» (%U)", string(p), p)
		}
	}
	return err
}

// inDoubleQuotedAttributeCodeSpan reads and writes runes inside of a double-quoted HTML attribute value. When it returns, the next rune to be read will be the one after the closing double quote.
func inDoubleQuotedAttributeValue(s *state) error {
	return inSpanEndingWithSingleUnescapedRune(s, '"')
}

// inSingleQuotedAttributeCodeSpan reads and writes runes inside of a single-quoted HTML attribute value. When it returns, the next rune to be read will be the one after the closing single quote.
func inSingleQuotedAttributeValue(s *state) error {
	return inSpanEndingWithSingleUnescapedRune(s, '\'')
}

// inUnquotedAttributeValue reads and writes runes until
func inUnquotedAttributeValue(s *state) error {

	// Note that this is more peek-heavy than the very-similar in{Single,Double}QuotedAttributeValue functions. This is because we want to end the function when the first not-part-of-the-value rune shows up. For single- and double-quoted values this is the predictable ' or ", but for unquoted attribute values it could be very different, like either whitespace or a >. We want to end the function when the first unpredictable rune shows up in the input and leave it to be read.
	for {
		p, err := s.peekRune()
		if err != nil {
			return err
		}

		if !isLegalHTMLAttributeValueUnquoted(p) {
			break
		}

		s.writeRune(s.mustReadRune())
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
	showHelp := flag.Bool("h", false, "Show help")

	flag.Parse()

	if showHelp != nil && *showHelp {
		flag.PrintDefaults()
		os.Exit(0)
	}

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

	whenceContents, err := io.ReadAll(whence)
	if err != nil {
		log.Fatalln("Something went wrong when reading input: ", err)
	}

	whenceReader := bytes.NewReader(whenceContents)

	if continueRewriteThings {
		// now that we’ve got the input all slurped up, let’s set up the out piping

		whither, err = os.OpenFile(flag.Args()[0], os.O_WRONLY|os.O_TRUNC, 0755) // BUG(adiabatic): cargo-culting the “0755”; I don’t understand masks
		if err != nil {
			log.Printf("Couldn’t open file «%s»: %s", flag.Args()[0], err)
		}

	}

	N, err := Educate(whither, whenceReader)
	if err != nil {
		log.Printf("%v bytes written before an error occurred: %v", N, err)
		os.Exit(1)
	}

	// stdout doesn’t like being synced, so don’t do it
	if whither != os.Stdout {
		err = whither.Sync()
		if err != nil {
			log.Printf("couldn’t flush to destination: %v", err)
			os.Exit(2)
		}
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
		R32: []unicode.Range32{ // go vet needs the Lo/Hi/Stride as of 2019-06-09
			{Lo: 0x1fffe, Hi: 0x1ffff, Stride: 1},
			{Lo: 0x2fffe, Hi: 0x2ffff, Stride: 1},
			{Lo: 0x3fffe, Hi: 0x3ffff, Stride: 1},
			{Lo: 0x4fffe, Hi: 0x4ffff, Stride: 1},
			{Lo: 0x5fffe, Hi: 0x5ffff, Stride: 1},
			{Lo: 0x6fffe, Hi: 0x6ffff, Stride: 1},
			{Lo: 0x7fffe, Hi: 0x7ffff, Stride: 1},
			{Lo: 0x8fffe, Hi: 0x8ffff, Stride: 1},
			{Lo: 0x9fffe, Hi: 0x9ffff, Stride: 1},
			{Lo: 0xafffe, Hi: 0xaffff, Stride: 1},
			{Lo: 0xbfffe, Hi: 0xbffff, Stride: 1},
			{Lo: 0xcfffe, Hi: 0xcffff, Stride: 1},
			{Lo: 0xdfffe, Hi: 0xdffff, Stride: 1},
			{Lo: 0xefffe, Hi: 0xeffff, Stride: 1},
			{Lo: 0xffffe, Hi: 0xfffff, Stride: 1},
			{Lo: 0x10fffe, Hi: 0x10ffff, Stride: 1},
		},
	}

	return !unicode.In(r, &table)
}

func isLegalHTMLAttributeValueUnquoted(r rune) bool {
	if isASCIIWhitespace(r) {
		return false
	}

	switch r {
	case '"', '\'', '=', '<', '>', '`':
		return false
	}

	// Ordinarily I’d actually check here to see if the rune is legal in an quoted attribute value, but The Standard says:
	//
	// “Attribute values are a mixture of text and character references, except with the additional restriction that the text cannot contain an ambiguous ampersand.”
	// — https://html.spec.whatwg.org/multipage/syntax.html#attributes-2
	//
	// So, like, anything goes? I guess so.
	return true

}
