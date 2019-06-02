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

func TestStrings(t *testing.T) {
	rows := []Row{
		// Absolute basics
		{"", ""},
		{" ", " "},
		{"hello", "hello"},
		/*
			// Backslashy things
			{
				"Some Europeans use \\` instead of ' when they're typing in English.",
				"Some Europeans use \\` instead of ‘ when they’re typing in English.",
			},
		*/
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

		// Single-quoty things
		{"Maybe I'd like lunch.", "Maybe I’d like lunch."},
		{"I like 'scare quotes'.", "I like ‘scare quotes’."},

		// Handle uninteresting HTML elements sensibly
		{
			`"What's it called? Dymaxion margarita?" "Close. <i>Dymondia margaretae</i>."`,
			"“What’s it called? Dymaxion margarita?” “Close. <i>Dymondia margaretae</i>.”",
		},
	}

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
