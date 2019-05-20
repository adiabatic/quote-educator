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
		{"I like \"sarcasm quotes\".", "I like “sarcasm quotes”."},
		{"I like \"American sarcasm quotes.\"", "I like “American sarcasm quotes.”"},
		{`"Who?" "He." "Whom?" "Him."`, `“Who?” “He.” “Whom?” “Him.”`},
		{"Maybe I'd like lunch.", "Maybe I’d like lunch."},
		{"I like 'scare quotes'.", "I like ‘scare quotes’."},
	}

	for _, row := range rows {
		t.Run(row.In, func(t *testing.T) {
			got, err := quotes.EducateString(row.In)
			if err != nil {
				t.Error(err)
			}
			if got != row.Want {
				t.Errorf("Expected «%s». got: «%s»", row.Want, got)
			}
		})
	}
}
