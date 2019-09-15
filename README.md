# Quote Educator

I like having curly quotes in my Markdown files. The trouble is, Markdown files frequently have have quote marks in them that _shouldn’t_ be curled. Ever seen travesties like `print(“Hello, world!”)` in a blog post? A _good_ quote curler wouldn’t do that.

`quote-educator` assumes your input is in Markdown, possibly with HTML in it.

By default, `quote-educator` reads from standard input and writes to standard output, with any errors or weirdness logged to standard error. If you trust `quote-educator` to not mess up your files (and/or have the files in source control), run <code>quote-educator -w <var>filename</var></code> to rewrite the file with curly quotes.

## Hacking

- Prefer `r` as a variable name for a rune you’ve read.
- Prefer `p` as a variable name for a rune you’ve peeked at.
- Prefer `o` as a variable name for a rune you’ve dug out of s.previousRune().
- Corollary: If you ever find yourself writing a variable named `p` to the output buffer, think long and hard whether that’s the right idea.
- Prefer `s.whatDo[p]` to `s.whatDo[r]`. You shouldn’t need to unread runes. For consistency’s sake, avoid structuring your rune-getting in such a way that you’ll need to unread runes.
