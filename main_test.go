package main_test

import (
	"testing"

	quotes "github.com/adiabatic/quote-educator"
)

type Row struct {
	In   string
	Want string
}

func TestStrings(t *testing.T) {
	rows := []Row{
		{"", ""},
		{" ", " "},
		{"hello", "hello"},

		// Double-quoty things
		{"I like \"sarcasm quotes\".", "I like “sarcasm quotes”."},
		{"I like \"American sarcasm quotes.\"", "I like “American sarcasm quotes.”"},
		{`"Who?" "He." "Whom?" "Him."`, `“Who?” “He.” “Whom?” “Him.”`},
		{
			`“I start fancy but end sloppy." "Oh, really?"`,
			`“I start fancy but end sloppy.” “Oh, really?”`,
		},
		{
			`"I get better with age.” "Like a cheese, then?"`,
			`“I get better with age.” “Like a cheese, then?”`,
		},

		// Single-quoty things
		{"Maybe I'd like lunch.", "Maybe I’d like lunch."},
		{"I like 'scare quotes'.", "I like ‘scare quotes’."},

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

		// Hey! Teacher! Leave my code alone
		{
			"I'd like to show you my first:\n\n```\nprint 'Hello, world!'\n```\n\nWasn't that difficult?",
			"I’d like to show you my first:\n\n```\nprint 'Hello, world!'\n```\n\nWasn’t that difficult?",
		},
	}

	for _, row := range rows {
		t.Run(row.In, func(t *testing.T) {
			got, err := quotes.EducateString(row.In)
			if err != nil {
				t.Error(err)
			}
			if got != row.Want {
				t.Errorf("\nexpected: «%s»\ngot:      «%s»", row.Want, got)
			}
		})
	}
}
