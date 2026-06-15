// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"
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

	s.whatDo['['] = atLeftBracket
	s.whatDo[']'] = atRightBracket

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

// atLineStart reports whether the output buffer currently sits at the start of a
// line, allowing up to three spaces of indentation (as CommonMark does before a
// link reference definition). It inspects what has already been written, so call
// it before writing the rune under consideration.
func (s *state) atLineStart() bool {
	bs := s.w.Bytes()
	for spaces := 0; ; {
		r, size := utf8.DecodeLastRune(bs)
		switch {
		case size == 0:
			return true // beginning of output
		case r == '\n':
			return true
		case r == ' ' && spaces < 3:
			spaces++
			bs = bs[:len(bs)-size]
		default:
			return false
		}
	}
}

// looksLikeReferenceDefinition reports whether the text just past the current
// offset — which sits right after a [ that began a line — is a CommonMark link
// reference definition: a non-empty label closed by ]: and followed, on the same
// line, by a real destination (and an optional title). It does not consume input.
//
// The destination check matters: without it, any line starting like [label]: —
// a pandoc footnote ([^1]: prose), a [speaker]: transcript line, an unterminated
// quote — would be mistaken for a definition and have its prose quotes left
// straight, or worse, orphan a quote span opened on an earlier line. We only
// suppress curling on lines that really are definitions.
func (s *state) looksLikeReferenceDefinition() bool {
	const maxScan = 4096 // labels and destinations are short; the line ends fast
	buf := make([]byte, maxScan)
	n, _ := s.r.ReadAt(buf, s.currentOffset())
	buf = buf[:n]

	// Locate the label's closing ], honoring backslash escapes, refusing to cross
	// a line break, and requiring at least one non-whitespace byte in the label.
	labelHasContent := false
	i := 0
	for ; i < len(buf); i++ {
		switch buf[i] {
		case '\\':
			i++ // also skip the escaped byte
			labelHasContent = true
			continue
		case '\n':
			return false
		case ']':
			// fall through to the break below
		default:
			if !isASCIIWhitespace(rune(buf[i])) {
				labelHasContent = true
			}
			continue
		}
		break
	}
	if i >= len(buf) || buf[i] != ']' || !labelHasContent {
		return false
	}
	i++ // past the ]

	// The ] must be immediately followed by :.
	if i >= len(buf) || buf[i] != ':' {
		return false
	}
	i++ // past the :

	// Whatever remains on this line must parse as a destination and optional title.
	rest := buf[i:]
	if nl := bytes.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[:nl]
	}
	return isReferenceTail(rest)
}

// isReferenceTail reports whether line — the text after a [label]: on a single
// line, with any trailing line break removed — is a well-formed link reference
// definition tail: optional whitespace, a non-empty destination, then an optional
// whitespace-separated title, then only trailing whitespace.
func isReferenceTail(line []byte) bool {
	i := skipASCIIWhitespaceBytes(line, 0)
	if i == len(line) {
		return false // a definition must have a destination
	}

	if line[i] == '<' {
		// An angle-bracketed destination runs to the matching >, with no < or line
		// break inside; a backslash escapes the next byte.
		i++
		closed := false
		for i < len(line) && !closed {
			switch line[i] {
			case '\\':
				i += 2
			case '<':
				return false
			case '>':
				i++
				closed = true
			default:
				i++
			}
		}
		if !closed {
			return false
		}
	} else {
		// A bare destination is a non-empty run of non-whitespace.
		start := i
		for i < len(line) && !isASCIIWhitespace(rune(line[i])) {
			if line[i] == '\\' && i+1 < len(line) {
				i += 2
				continue
			}
			i++
		}
		if i == start {
			return false
		}
	}

	// With no title, only trailing whitespace may follow the destination.
	rest := skipASCIIWhitespaceBytes(line, i)
	if rest == len(line) {
		return true
	}
	if rest == i {
		return false // a title must be set off from the destination by whitespace
	}

	// A title is delimited by "...", '...', or (...), with backslash escapes.
	var closer byte
	switch line[rest] {
	case '"':
		closer = '"'
	case '\'':
		closer = '\''
	case '(':
		closer = ')'
	default:
		return false
	}
	for j := rest + 1; j < len(line); j++ {
		switch line[j] {
		case '\\':
			j++ // also skip the escaped byte
		case closer:
			return skipASCIIWhitespaceBytes(line, j+1) == len(line)
		}
	}
	return false // unterminated title
}

func skipASCIIWhitespaceBytes(b []byte, i int) int {
	for i < len(b) && isASCIIWhitespace(rune(b[i])) {
		i++
	}
	return i
}

// atLeftBracket reads an assumed-to-exist [. At the start of a line it may begin a
// CommonMark link reference definition — [label]: destination — whose URL must not
// have its quotes curled, just like an inline-link destination. When it does,
// inReferenceDefinition copies the rest of the line through untouched. A [ anywhere
// else (inline-link text, a [1] footnote marker, ordinary prose) is left to normal
// processing, which still curls the link text and lets atRightBracket guard any
// inline ](url) destination.
func atLeftBracket(s *state) error {
	r := s.mustReadRune()
	if r != '[' {
		return fmt.Errorf("expecting a left bracket ([). got: «%s» (%U)", string(r), r)
	}

	if s.atLineStart() && s.looksLikeReferenceDefinition() {
		s.writeRune(r)
		return inReferenceDefinition(s)
	}

	return s.writeRune(r)
}

// inReferenceDefinition copies a link reference definition through without curling
// any quotes. The opening [ has already been written; this copies the label, its
// closing ], the : that follows, and the rest of the line — destination and any
// title — verbatim. Curling a quote in the destination would silently break the
// link, and a curled quote around a title would stop CommonMark from recognizing
// it, so the whole definition line is left alone. (A title carried onto a following
// line is out of scope and gets educated normally.)
//
// When it returns, the next rune to be read is the line-ending \n (or the input is
// exhausted).
func inReferenceDefinition(s *state) error {
	// Copy the label through its closing ]. looksLikeReferenceDefinition already
	// confirmed a ] (immediately followed by :) lies ahead on this line.
	for {
		r, err := s.readRune()
		if err != nil {
			return err
		}
		s.writeRune(r)

		if r == '\\' {
			if err := s.AdvanceBy(1); err != nil {
				return err
			}
			continue
		}
		if r == ']' {
			break
		}
	}

	// Copy the : and the destination (and any title) to the end of the line.
	return s.AdvanceUntil("\n")
}

// atRightBracket reads an assumed-to-exist ]. If the ] is immediately followed by a (, it begins a Markdown inline-link destination like ](https://example.com), so it hands off to inLinkDestination to copy the URL (and any title) through untouched. Curling a quote inside a URL silently breaks the link, so we never do it. A bare ] not followed by ( is ordinary text.
//
// We treat any ]( as a link destination without checking for a matching opening [, mirroring the cheap heuristics elsewhere here (any ` starts a code span, any <letter an HTML tag). The cost is that a stray ]( in prose leaves its parenthesized quotes straight; that is rare and self-limited to the matching ).
func atRightBracket(s *state) error {
	r := s.mustReadRune()
	if r != ']' {
		return fmt.Errorf("expecting a right bracket (]). got: «%s» (%U)", string(r), r)
	}

	s.writeRune(r)

	if !s.PeekEquals("(") {
		return nil
	}

	s.writeRune(s.mustReadRune()) // the (
	return inLinkDestination(s)
}

// inLinkDestination reads and writes the body of a Markdown inline-link destination — everything after the opening ( up to and including the matching ) — without curling any quotes. Parentheses may nest, as CommonMark allows in destinations like ](https://en.wikipedia.org/wiki/Foo_(bar)), so we track depth; a backslash escapes the next rune, so \) does not close the destination.
//
// A destination may carry a title set off from the URL by whitespace, e.g. ](url "the title"). A "…" or '…' title can itself contain unescaped parentheses, so we consume it as a quoted span rather than letting those parens skew the depth count. (A (…)-delimited title is just more nested parens, already handled.) The whitespace test is what distinguishes a title’s opening quote from a quote inside the URL, like ](http://example.com/?q="x").
//
// When it returns, the next rune to be read will be the one right after the closing ).
func inLinkDestination(s *state) error {
	depth := 1
	var prev rune
	for {
		r, err := s.readRune()
		if err != nil {
			return err
		}

		s.writeRune(r)

		switch r {
		case '\\':
			r, err = s.readRune()
			if err != nil {
				return err
			}
			s.writeRune(r)
		case '"', '\'':
			if isASCIIWhitespace(prev) {
				if err := inSpanEndingWithSingleUnescapedRune(s, r); err != nil {
					return err
				}
			}
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return nil
			}
		}

		prev = r
	}
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
			err = fmt.Errorf("got some weird rune that’s starting an HTML attribute value: «%s» (%U)", string(p), p)
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

// WouldChange reports whether educating src would alter it — the predicate
// behind --check. It returns true when src isn’t already educated.
func WouldChange(src []byte) (bool, error) {
	var educated bytes.Buffer
	if _, err := Educate(&educated, bytes.NewReader(src)); err != nil {
		return false, err
	}
	return !bytes.Equal(src, educated.Bytes()), nil
}

const longHelp = `Educates straight ASCII quotes (' ") into typographic curly quotes (‘ ’ “ ”) in Markdown text.

By default, reads Markdown from stdin and writes the educated result to stdout.
With -w, reads the given file and rewrites it in place instead.
With --check, reads stdin (or the given file) and writes nothing; it exits 0 if
the input is already educated and 1 if running the tool would change it.
(--check and -n are mutually exclusive.)

Idempotent: running the tool twice produces the same output as running it once,
since already-curly quotes are left alone. It is safe to re-run.

Scope: the tool is Markdown-aware and does NOT alter quotes inside:
  • YAML frontmatter (between leading --- fences)
  • inline code spans (` + "`like this`" + `)
  • fenced code blocks (` + "``` ... ```" + `)
  • inline-link destinations, e.g. the URL in ](http://example.com/?q="x")
  • reference-style link definitions, e.g. [ref]: http://example.com/?q="x"
    (the whole definition line is left alone, including any title)
  • autolinks, e.g. <http://example.com/?q="x">

Exit codes:
  0  success (with --check, the input is already educated)
  1  --check only: the input is not yet educated; running without --check
     would change it
  2  error: bad usage, unreadable input, or unwritable output`

const examples = `  # Educate text piped in on stdin, writing the result to stdout
  echo 'He said "hi" to the world.' | quote-educator

  # Educate a file and print the result to stdout (file is left unchanged)
  quote-educator < README.md

  # Rewrite a single file in place (file arguments are only read with -w or --check)
  quote-educator -w README.md

  # Exit 0 if a file is already educated, 1 if running the tool would change it
  quote-educator --check README.md

  # Same check on stdin
  cat README.md | quote-educator --check`

// Exit codes follow the diff(1)/grep(1) convention: 0 means success, 1 is
// reserved for --check reporting that changes are pending, and 2 means an
// error occurred. Keeping changes-pending (1) distinct from errors (2) lets
// --check callers tell “not yet educated” apart from “the tool broke”.
const (
	exitOK      = 0 // success; with --check, the input is already educated
	exitChanges = 1 // --check only: educating would change the input
	exitError   = 2 // bad usage, unreadable input, or unwritable output
)

func main() {
	var (
		rewriteInPlace  bool
		addExtraNewline bool
		checkOnly       bool
	)

	rootCmd := &cobra.Command{
		Use:           "quote-educator [flags] [file]",
		Short:         "Educate straight ASCII quotes into typographic curly quotes in Markdown text.",
		Long:          longHelp,
		Example:       examples,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, args []string) {
			run(args, rewriteInPlace, addExtraNewline, checkOnly)
		},
	}

	flags := rootCmd.Flags()
	flags.BoolVarP(&rewriteInPlace, "rewrite-in-place", "w", false, "rewrite the source file in place instead of writing to stdout (requires exactly one file argument)")
	flags.BoolVarP(&addExtraNewline, "add-newline", "n", false, "add an extra newline at the end")
	flags.BoolVar(&checkOnly, "check", false, "write nothing; exit 0 if input is already educated, 1 if it would change, 2 on error")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(exitError)
	}
}

func run(args []string, rewriteInPlace, addExtraNewline, checkOnly bool) {
	var whence io.Reader = os.Stdin
	var whither = os.Stdout

	if checkOnly {
		if addExtraNewline {
			log.Println("--check and -n are mutually exclusive: --check writes nothing, so there is nothing to add a newline to")
			os.Exit(exitError)
		}
		switch len(args) {
		case 0:
			// read from stdin
		case 1:
			f, err := os.Open(args[0])
			if err != nil {
				log.Printf("Could not open file named “%s”: %v\n", args[0], err)
				os.Exit(exitError)
			}
			defer f.Close()
			whence = f
		default:
			log.Println("Must specify at most one file with --check")
			os.Exit(exitError)
		}

		original, err := io.ReadAll(whence)
		if err != nil {
			log.Println("Something went wrong when reading input: ", err)
			os.Exit(exitError)
		}

		changed, err := WouldChange(original)
		if err != nil {
			log.Println("Something went wrong while educating: ", err)
			os.Exit(exitError)
		}
		if changed {
			os.Exit(exitChanges)
		}
		os.Exit(exitOK)
	}

	continueRewriteThings := false
	if rewriteInPlace {
		switch len(args) {
		case 0:
			log.Println("Must specify a file to overwrite with -w")
			os.Exit(exitError)
		case 1:
			// continue
		default:
			log.Println("Must specify only one file to overwrite with -w")
			os.Exit(exitError)
		}

		continueRewriteThings = true
		var err error
		whence, err = os.Open(args[0])
		if err != nil {
			log.Printf("Could not open file named “%s” for both reading and writing: %v\n", args[0], err)
			os.Exit(exitError)
		}
	}

	whenceContents, err := io.ReadAll(whence)
	if err != nil {
		log.Println("Something went wrong when reading input: ", err)
		os.Exit(exitError)
	}

	whenceReader := bytes.NewReader(whenceContents)

	if continueRewriteThings {
		// now that we’ve got the input all slurped up, let’s set up the out piping

		// Mode is ignored without O_CREATE: the file already exists (opened above) and O_TRUNC keeps its mode.
		whither, err = os.OpenFile(args[0], os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			log.Printf("Couldn’t open file «%s» for writing: %v", args[0], err)
			os.Exit(exitError)
		}

	}

	N, err := Educate(whither, whenceReader)
	if err != nil {
		log.Printf("%v bytes written before an error occurred: %v", N, err)
		os.Exit(exitError)
	}

	if addExtraNewline {
		n, err := whither.WriteString("\n")
		if n != 1 || err != nil {
			log.Printf("Could not slap on one final newline. Error, if any: %v", err)
			os.Exit(exitError)
		}
	}

	// stdout doesn’t like being synced, so don’t do it
	if whither != os.Stdout {
		err = whither.Sync()
		if err != nil {
			log.Printf("couldn’t flush to destination: %v", err)
			os.Exit(exitError)
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
