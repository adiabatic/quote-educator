// SPDX-License-Identifier: AGPL-3.0-only

package main_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	quotes "github.com/adiabatic/quote-educator"
)

// EducateString is a convenience function for running Educate on strings.
func EducateString(s string) (string, error) {
	br := bytes.NewReader([]byte(s))
	out := &strings.Builder{}

	_, err := quotes.Educate(out, br)
	if err != nil && err != io.EOF {
		return "", err
	}

	return out.String(), nil
}

type Row struct {
	In   string
	Want string
}

var rows = []Row{
	// Absolute basics
	{"", ""},
	{" ", " "},
	{"hello", "hello"},

	// Don’t swallow trailing newlines
	{"hello\n", "hello\n"},
	{"hello\n\n", "hello\n\n"},

	// Don’t swallow trailing spaces, either
	{"I'd ", "I’d "},

	// Backslashy things
	{
		"Some Europeans use \\` instead of ' when they're typing in English.",
		"Some Europeans use \\` instead of ‘ when they’re typing in English.",
	},

	// Double-quoty things
	{
		`I like "sarcasm quotes".`,
		`I like “sarcasm quotes”.`,
	},
	{
		`I like "American sarcasm quotes."`,
		"I like “American sarcasm quotes.”",
	},
	{
		`"Who?" "He." "Whom?" "Him."`,
		`“Who?” “He.” “Whom?” “Him.”`,
	},
	{
		`“I start fancy but end sloppy." "Oh, really?"`,
		`“I start fancy but end sloppy.” “Oh, really?”`,
	},
	{
		`"I get better with age.” "Like a cheese, then?"`,
		`“I get better with age.” “Like a cheese, then?”`,
	},

	// Handle triple nesting
	{
		`"'Tell him I said "ow"'. Gotcha!"`,
		`“‘Tell him I said “ow”’. Gotcha!”`,
	},

	// Single-quoty things
	{"Maybe I'd like lunch.", "Maybe I’d like lunch."},
	{"I like 'scare quotes'.", "I like ‘scare quotes’."},

	// Ensure apostrophes after single quotes do the right thing
	{
		"'I like traffic lights' isn't an example of an interrogative sentence. 'Is this a sheep?' is.",
		"‘I like traffic lights’ isn’t an example of an interrogative sentence. ‘Is this a sheep?’ is.",
	},

	// Ensure apostrophes in single quotes do the right thing
	{
		"'So you're saying I can't take sheep on the aeroplane?'",
		"‘So you’re saying I can’t take sheep on the aeroplane?’",
	},

	// Things with hyphens
	{"Ob-La-Di, Ob-La-Da", "Ob-La-Di, Ob-La-Da"},

	// YAML front matter
	{
		"---\ntitle: 'Zelda: Breath of the Wild vignettes'\n---\n\nYou can't just fall on a horse.\n",
		"---\ntitle: 'Zelda: Breath of the Wild vignettes'\n---\n\nYou can’t just fall on a horse.\n",
	},

	// Horizontal rules aren’t YAML front matter
	{
		"Let's take a breather.\n\n---\n\nWasn't that nice?.",
		"Let’s take a breather.\n\n---\n\nWasn’t that nice?.",
	},

	// Ignore quote marks in code spans
	{
		"Let's consider \"Hello, World\" in Python. It's merely `print(\"Hello, World\")`. Now let's consider what that looks like in Java…",
		"Let’s consider “Hello, World” in Python. It’s merely `print(\"Hello, World\")`. Now let’s consider what that looks like in Java…",
	},

	// Hey! Teacher! Leave my code alone
	{
		"I'd like to show you my first:\n\n```\nprint 'Hello, world!'\n```\n\nWasn't that difficult?",
		"I’d like to show you my first:\n\n```\nprint 'Hello, world!'\n```\n\nWasn’t that difficult?",
	},

	// Gotta curl quotes after the code span is over.
	{
		"`⌘⇥` isn't very different from Windows, but…",
		"`⌘⇥` isn’t very different from Windows, but…",
	},

	// Backslashed backticks in code spans
	{
		"`⌘\\`` isn't easy to get used to",
		"`⌘\\`` isn’t easy to get used to",
	},

	// More fun with backslashes and backticks
	{
		"\"`\\`ls\\` # I don't know what I'm doing`\" was the comment he'd written all those years ago?",
		"“`\\`ls\\` # I don't know what I'm doing`” was the comment he’d written all those years ago?",
	},

	// Don’t curl quotes inside an inline link’s URL — it would break the link
	{
		`See [the page](http://example.com/?q="x") for more.`,
		`See [the page](http://example.com/?q="x") for more.`,
	},

	// …but still curl quotes in the link text and the surrounding prose
	{
		`I "love" [this "page"](http://example.com/?q="x"), don't you?`,
		`I “love” [this “page”](http://example.com/?q="x"), don’t you?`,
	},

	// A link destination may contain balanced parentheses
	{
		`[wiki](https://en.wikipedia.org/wiki/Foo_(bar)) isn't broken.`,
		`[wiki](https://en.wikipedia.org/wiki/Foo_(bar)) isn’t broken.`,
	},

	// A link title is left alone, even when it contains parentheses that would
	// otherwise look like they close the destination early
	{
		`See [docs](http://example.com "Smith (2020)") and "after".`,
		`See [docs](http://example.com "Smith (2020)") and “after”.`,
	},

	// Escaped quotes inside a link title don’t end it prematurely
	{
		`Read [docs](http://example.com "the \"best\" page") now.`,
		`Read [docs](http://example.com "the \"best\" page") now.`,
	},

	// A quote in the URL itself (not a title) is still left straight
	{
		`Open [it](http://example.com/?q="x" "the title") please.`,
		`Open [it](http://example.com/?q="x" "the title") please.`,
	},

	// A link sitting inside an open quote: curl the quote, not the URL
	{
		`I "love [x](http://y/?q="z") stuff" here.`,
		`I “love [x](http://y/?q="z") stuff” here.`,
	},

	// A bare ] with no following ( is ordinary text
	{
		`I'd say [1] is "fine".`,
		`I’d say [1] is “fine”.`,
	},

	// Don’t curl quotes inside an autolink
	{
		`Visit <http://example.com/?q="x"> today.`,
		`Visit <http://example.com/?q="x"> today.`,
	},

	// Handle uninteresting HTML elements sensibly
	{
		`"What's it called? Dymaxion margarita?" "Close. <i>Dymondia margaretae</i>."`,
		"“What’s it called? Dymaxion margarita?” “Close. <i>Dymondia margaretae</i>.”",
	},

	// Handle intact <code> elements
	{
		"<code>snprintf(buffer, ∆izeof(buffer), \"%s\", string);</code>",
		"<code>snprintf(buffer, ∆izeof(buffer), \"%s\", string);</code>",
	},

	// Handle uninteresting weirdly-spaced HTML elements sensibly
	{
		"<code >Console.WriteLine(\"Hello, world!\");</code>",
		"<code >Console.WriteLine(\"Hello, world!\");</code>",
	},

	// Handle empty attributes
	{
		"<p hidden>I'm a spooky ghost. OooOoOoooo.</p>",
		"<p hidden>I’m a spooky ghost. OooOoOoooo.</p>",
	},

	// Handle space after empty attributes
	{
		"<input disabled  >∂sn't this illegal HTML?</input>",
		"<input disabled  >∂sn’t this illegal HTML?</input>",
	},

	// Handle double-quoted attributes
	{
		`<abbr title="YAML Ain't Markup Language">YAML</abbr> isn't bad.`,
		`<abbr title="YAML Ain't Markup Language">YAML</abbr> isn’t bad.`,
	},

	// Handle double-quoted attributes with backslashed escapes
	{
		`<a title="Nick \"Goose\" Bradshaw">Anthony Edwards's role</a>`,
		`<a title="Nick \"Goose\" Bradshaw">Anthony Edwards’s role</a>`,
	},

	// Handle single-quoted attributes
	{
		"<label placeholder='your dog\\'s name'>Fido of Green's Hill",
		"<label placeholder='your dog\\'s name'>Fido of Green’s Hill",
	},

	// Handle unquoted attributes
	{
		"<h2 id=jacks-oatmeal>Jack's Oatmeal</h2>",
		"<h2 id=jacks-oatmeal>Jack’s Oatmeal</h2>",
	},

	// Handle multiple attributes
	{
		`<abbr id=yaml title="YAML Ain't Markup Language">YAML</abbr> ain't the worst.`,
		`<abbr id=yaml title="YAML Ain't Markup Language">YAML</abbr> ain’t the worst.`,
	},
}

func TestStrings(t *testing.T) {
	for _, row := range rows {
		t.Run(row.In, func(t *testing.T) {
			got, err := EducateString(row.In)
			if err != nil {
				t.Error(err)
			}
			if got != row.Want {
				t.Errorf("\nsource:   «%s»\nexpected: «%s»\ngot:      «%s»", row.In, row.Want, got)
			}
		})
	}
}

// TestWouldChange exercises the --check predicate against the shared corpus:
// every Want is already educated (no change), and every In changes exactly when
// it differs from its educated form.
func TestWouldChange(t *testing.T) {
	for _, row := range rows {
		t.Run(row.In, func(t *testing.T) {
			clean, err := quotes.WouldChange([]byte(row.Want))
			if err != nil {
				t.Fatal(err)
			}
			if clean {
				t.Errorf("WouldChange(%q) = true; already-educated input must report no change", row.Want)
			}

			got, err := quotes.WouldChange([]byte(row.In))
			if err != nil {
				t.Fatal(err)
			}
			if want := row.In != row.Want; got != want {
				t.Errorf("WouldChange(%q) = %v; want %v", row.In, got, want)
			}
		})
	}
}

// TestIdempotent locks in the documented contract that educating already-curly
// output is a no-op, so re-running the tool is safe. Each row’s Want is the
// canonical educated form, so feeding it back through Educate must return it
// unchanged.
func TestIdempotent(t *testing.T) {
	for _, row := range rows {
		t.Run(row.Want, func(t *testing.T) {
			got, err := EducateString(row.Want)
			if err != nil {
				t.Error(err)
			}
			if got != row.Want {
				t.Errorf("\nnot idempotent:\ninput:  «%s»\ngot:    «%s»", row.Want, got)
			}
		})
	}
}
