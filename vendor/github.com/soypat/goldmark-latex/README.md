# goldmark-latex
A LaTeX renderer for [Goldmark](https://github.com/yuin/goldmark). Produce `.tex` files from markdown.

This renderer seeks to be as extensible as Goldmark itself. Please file an issue if it does not meet your requirements.

## Results
So far this implementation renders the CommonMark specification with the exception of embedded HTML. It does have some bugs related to undefined ASCII sequences. Any help is appreciated.

![result](https://user-images.githubusercontent.com/26156425/188299284-8dd2fca1-dc50-4574-8128-c78017b42e73.png)

## Extensions

### Inline Math

`$...$` expressions are passed through as LaTeX inline math. The closing `$` must appear on the same line. An unmatched `$` is escaped to `\$` as usual.

| Markdown | LaTeX |
|---|---|
| `$x + y$` | `$x + y$` |
| `See $E=mc^2$.` | `See $E=mc^2$.` |
| `cost is $5` | `cost is \$5` |

Register `InlineMathParser` to enable this feature.

### GFM Tables

Pipe tables are rendered as `tabular` environments. Column alignment from the delimiter row (`:---`, `:---:`, `---:`) maps to `l`, `c`, `r` in the column spec.

Input:

```markdown
| Left | Center | Right |
| :--- | :----: | ----: |
| a    | b      | c     |
```

Output:

```latex
\begin{tabular}{lcr}
\hline
Left & Center & Right \\
\hline
a & b & c \\
\hline
\end{tabular}
```

### Table Captions

Enabled via `Config.EnableTableCaptions`. A paragraph beginning with `: ` immediately after a table is treated as a caption and wraps the table in a `table` float.

Input:

```markdown
| A | B |
| - | - |
| 1 | 2 |

: My caption
```

Output:

```latex
\begin{table}[h!]
\centering
\begin{tabular}{ll}
\hline
A & B \\
\hline
1 & 2 \\
\hline
\end{tabular}
\caption{My caption}
\end{table}
```

### Figures

Markdown images render as `figure` floats wrapping `\includegraphics`. The alt text becomes the figure caption (omitted when empty). The destination is written verbatim so file paths with characters such as `_` are preserved. Dangerous URLs are dropped unless `Config.Unsafe` is set. Requires the `graphicx` package (included in the default preamble).

Input:

```markdown
![A cute cat](cat_photo.png)
```

Output:

```latex
\begin{figure}[h]
\centering
\includegraphics[width=\textwidth]{cat_photo.png}
\caption{A cute cat}
\end{figure}
```

### Raw LaTeX Passthrough

A fenced code block with the pandoc-style `{=latex}` info string emits its contents verbatim, letting you inject arbitrary LaTeX (e.g. `tikzpicture`, custom environments). Because this is arbitrary code injection, it is gated behind `Config.Unsafe` (the `-unsafe` flag in `md2latex`); without it the block is dropped with a comment.

Input:

````markdown
```{=latex}
\begin{tikzpicture}
\draw (0,0) -- (1,1);
\end{tikzpicture}
```
````

Output (with `Config.Unsafe`):

```latex
\begin{tikzpicture}
\draw (0,0) -- (1,1);
\end{tikzpicture}
```

### Citations and Bibliography

Enabled by registering `CitationParser`. Converts Pandoc-style `[@key]` inline citations to `\cite{key}`. Multiple keys are separated by `; @`.

| Markdown | LaTeX |
|---|---|
| `[@darwin1859]` | `\cite{darwin1859}` |
| `[@key1; @key2]` | `\cite{key1,key2}` |

The cite command can be overridden via `Config.CiteCmd` (e.g. `"citep"` for natbib's `\citep{}`).

A bibliography block is emitted before `\end{document}` when `Config.BibFile` is set:

```latex
\bibliographystyle{plain}
\bibliography{refs}
```

`Config.BibStyle` controls the style argument; it defaults to `"plain"`.

## md2latex program
This command converts a single markdown file to latex and writes to contents to a new .text file or to stdout.
