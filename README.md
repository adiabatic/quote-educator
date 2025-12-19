# Quote Educator

I like having curly quotes in my Markdown files. The trouble is, Markdown files frequently have have quote marks in them that _shouldn’t_ be curled. Ever seen travesties like `print(“Hello, world!”)` in a blog post? A _good_ quote curler wouldn’t do that.

`quote-educator` assumes your input is in Markdown, possibly with HTML in it.

By default, `quote-educator` reads from standard input and writes to standard output, with any errors or weirdness logged to standard error. If you trust `quote-educator` to not mess up your files (and/or have the files in source control), run <code>quote-educator -w <var>filename</var></code> to rewrite the file with curly quotes.

## Integration

### [Helix][]

I have `quote-educator` set up to be used [in my Helix config][hxcfg]. This works extra well with Helix’s `]g` (`goto_next_change`), provided your cursor is before (or at least outside) any text that’s changed according to Git.

[helix]: https://helix-editor.com/
[hxcfg]: https://github.com/adiabatic/helix-config/blob/master/config.toml

### the macOS [Shortcuts][] app

This is fairly useful as a Shortcut in the Shortcuts app. It looks like this on my machine, although you’ll have to change the path to where you installed `quote-educator` on your computer:

<picture><source src='doc/macos-shortcut/light.png' media='(prefers-color-scheme: light)'><img src='doc/macos-shortcut/dark.png' alt='A macOS capital-S Shortcut. Its name is “Curl Quotes”. Over on the right, in the Shortcut Details section, “Show in Spotlight” is checked. “Use as Quick Action” is checked. “Services Menu” is checked. “Provide Output” is checked. In the middle big part of the window, the Shortcut is set to receive Text from Quick Actions. If there is no input, it will Stop and Respond with “No input text”. The one real Action is “Run Shell Script”. The only line in the shell script is the full path to wherever you put the quote-educator binary. The shell is listed as ZSH. The input is set to Shortcut Input. Pass Input is set to standard input. The “Run as Administrator” box is unchecked. Finally, we come to the third and final step. It is set to Stop and Output the shell script result. If there’s nowhere to output, do nothing.'></picture>

You can then trigger it from right-clicking on some text, choosing `Services`, and then choosing `Curl Quotes`.

Or you can select some text and invoke this Shortcut from the Services menu underneath the name of whatever application you’re running.

Or, as of macOS 26.2, you can select some text, trigger Spotlight by pressing ⌘␣ (Command-space), typing as much of `Curl Quotes` as it takes to narrow down the list of options, and pressing `Return`.

[shortcuts]: https://support.apple.com/guide/shortcuts-mac/intro-to-shortcuts-apdf22b0444c/mac

## Hacking

- Prefer `r` as a variable name for a rune you’ve read.
- Prefer `p` as a variable name for a rune you’ve peeked at.
- Prefer `o` as a variable name for a rune you’ve dug out of s.previousRune().
- Corollary: If you ever find yourself writing a variable named `p` to the output buffer, think long and hard whether that’s the right idea.
- Prefer `s.whatDo[p]` to `s.whatDo[r]`. You shouldn’t need to unread runes. For consistency’s sake, avoid structuring your rune-getting in such a way that you’ll need to unread runes.
