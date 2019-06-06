# Quote Educator

I like having curly quotes in my Markdown files. The trouble is, Markdown files frequently have have quote marks in them that _shouldn’t_ be curled. Ever seen travesties like `print(“Hello, world!”)` in a blog post? A _good_ quote curler wouldn’t do that.

I make no claim that `quote-educator` is good, but it attempts to handle at least some of the edge cases that crop up in Markdown files.

## Hacking

- Prefer `r` as a variable name for a rune you’ve read.
- Prefer `p` as a variable name for a rune you’ve peeked at.
- Corollary: If you ever find yourself writing a variable named `p`, think long and hard whether that’s the right idea.
- Prefer `s.whatDo[p]` to `s.whatDo[r]`. You shouldn’t need to unread runes. For consistency’s sake, avoid structuring your rune-getting in such a way that you’ll need to unread runes.
