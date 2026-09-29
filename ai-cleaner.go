/*
 * Git: https://github.com/crypery
 * Author: https://crypery.com
 * License: GNU AGPL v3 (Affero GPL)
 */
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type PhraseReplacement struct {
	Pattern     *regexp.Regexp
	Replacement []byte
}

// phrasePattern builds a regexp that matches any of the alternatives as a
// whole word. Go's \b is ASCII-only (so it never matches Cyrillic), therefore
// boundaries are expressed with Unicode classes:
//
//	(line start or a non-letter separator) + phrase + (one or more separators, or line end)
//
// The leading separator is captured in group 1, the trailing one in group 2.
// The trailing boundary requires at least one separator (or line end), so a
// phrase can never match inside a longer token such as an identifier.
func phrasePattern(alternatives ...string) *regexp.Regexp {
	quoted := make([]string, len(alternatives))
	for i, a := range alternatives {
		quoted[i] = regexp.QuoteMeta(a)
	}
	return regexp.MustCompile(`(?i)(^|[^\p{L}\p{N}])(?:` + strings.Join(quoted, "|") + `)([\s.,;:!?]+|$)`)
}

// remove deletes the phrase and any trailing separator, keeping the leading
// one (so the text does not accumulate double spaces).
func remove(alternatives ...string) PhraseReplacement {
	return PhraseReplacement{Pattern: phrasePattern(alternatives...), Replacement: []byte("${1}")}
}

// replace swaps the phrase for replacement, keeping surrounding separators.
// Braced ${1}/${2} are used so Go does not parse "$1<text>" as a named
// backreference (which would silently drop the replacement text).
func replace(replacement string, alternatives ...string) PhraseReplacement {
	return PhraseReplacement{Pattern: phrasePattern(alternatives...), Replacement: []byte("${1}" + replacement + "${2}")}
}

var (
	// Regex to match all Unicode emoji characters and sequences
	// (including newer blocks like Symbols and Pictographs Extended-A,
	// keycaps, flags and skin-tone modifiers), plus ZWJ joiners, variation
	// selectors and combining keycap marks so that multi-codepoint emoji
	// sequences (e.g. 👨‍👩‍👧‍👦, 1️⃣, ©️) are removed as a whole.
	emojiRegex = regexp.MustCompile(`[\x{00A9}\x{00AE}\x{203C}\x{2049}\x{2122}\x{2139}\x{2194}-\x{21FF}\x{2300}-\x{23FF}\x{25A0}-\x{25FF}\x{2600}-\x{27BF}\x{2B00}-\x{2BFF}\x{3030}\x{303D}\x{3297}\x{3299}\x{200D}\x{20E3}\x{FE0F}\x{1F004}\x{1F0CF}\x{1F170}-\x{1F171}\x{1F17E}-\x{1F17F}\x{1F18E}\x{1F191}-\x{1F19A}\x{1F1E6}-\x{1F1FF}\x{1F200}-\x{1F202}\x{1F21A}\x{1F22F}\x{1F232}-\x{1F23A}\x{1F250}-\x{1F251}\x{1F300}-\x{1F64F}\x{1F680}-\x{1F6FF}\x{1F7E0}-\x{1F7EB}\x{1F7F0}\x{1F900}-\x{1F9FF}\x{1FA70}-\x{1FAFF}]`)

	// Regex to remove spaces before punctuation marks (. , : ; ! ?)
	spacesBeforePunctuation = regexp.MustCompile(`\s+([.,:;!?])`)

	// Regex to match dash-like characters: box-drawing light horizontal "─",
	// en dash "–", em dash "—" and horizontal bar "―", all normalized to "-".
	dashRegex = regexp.MustCompile(`[\x{2013}\x{2014}\x{2015}\x{2500}]`)

	// Regex to convert markdown bullet lists from asterisks (*) to hyphens (-)
	markdownListRegex = regexp.MustCompile(`^(\s*)\*\s+`)

	// Regex to collapse 3 or more consecutive newlines into 2 (one blank line)
	consecutiveNewlines = regexp.MustCompile(`\n{3,}`)

	// Comprehensive list of AI stop-phrases for replacement/suppression.
	// Order matters: longer/specific phrases go before shorter fragments,
	// so a longer marker is never eaten by a shorter prefix of it.
	aiPhrases = []PhraseReplacement{
		// --- Russian: introductory phrases and confirmations (wiped out) ---
		remove("конечно, вот", "конечно, ниже", "конечно, давайте", "конечно, сейчас", "конечно, могу", "без проблем, вот", "рад помочь, вот"),
		remove("вот обновленный код", "вот исправленный вариант", "вот исправленный код", "вот обновленный файл", "вот полный код", "вот готовый код", "вот готовый файл", "вот полный файл", "ниже приведен измененный вариант", "как и запрашивалось", "согласно вашему запросу", "по вашему требованию", "готовый скрипт выглядит следующим образом"),
		remove("внимательно изучив ваш код, я заметил", "я изучил ваш код", "я проверил ваш код", "я проанализировал ваш код"),
		remove("я исправил ошибку", "ошибка исправлена", "я исправил код", "код исправлен", "я исправил", "исправлено", "я обновил код", "код обновлен", "я обновил"),
		remove("теперь код работает корректно и без ошибок", "теперь код работает корректно", "код работает корректно", "все работает корректно", "теперь все работает", "все готово", "готово"),
		remove("если у вас возникнут дополнительные вопросы, не стесняйтесь обращаться", "не стесняйтесь обращаться", "если у вас возникнут вопросы", "если возникнут вопросы", "если появятся вопросы", "если у вас есть вопросы, обращайтесь"),
		remove("надеюсь, это поможет", "надеюсь, помог", "надеюсь, я помог", "надеюсь, ответил на ваш вопрос", "надеюсь, было полезно"),
		remove("как вы можете видеть", "как вы видите", "как видите", "как видно из кода", "как можно заметить", "как можно видеть", "как вы можете заметить"),
		remove("как я уже говорил", "как я уже упоминал", "как упоминалось ранее", "как было сказано ранее", "как я упоминал ранее", "как было сказано"),
		remove("давайте разберемся", "давайте разберем", "давайте рассмотрим", "разберем по шагам", "давайте посмотрим", "давайте посмотрим на", "пойдем по шагам"),
		remove("вот пример кода", "вот пример", "вот примеры", "вот код", "код ниже", "полный код ниже"),
		remove("конечно", "да, конечно", "без проблем", "рад помочь", "с удовольствием", "хороший вопрос", "отличный вопрос", "прекрасный вопрос", "понял вас"),

		// --- English: introductory phrases and confirmations (wiped out) ---
		remove("here is the updated code", "here is the corrected version", "the modified code is below", "the complete script is as follows", "as requested", "according to your request", "per your request", "here is the updated file", "here is the corrected code", "here is the full code", "here is the complete code", "here is the full file", "here is the code", "here is an example", "here's an example", "here is a simple example", "here is a complete example", "here's how", "here is how"),
		remove("i have updated the code", "i've updated the code", "i updated the code", "i have fixed the code", "i've fixed the code", "i fixed the code", "the code is fixed", "the issue is fixed", "the error is fixed", "i have fixed it", "i've fixed it", "i fixed it"),
		remove("i have tested the code", "i've tested the code", "i tested the code", "i ran the code", "i tested it", "i ran it", "it works now", "it should work now", "it's working now", "it is working now", "everything works now", "everything is working now"),
		remove("if you have any further questions, feel free to ask", "if you have any questions, feel free to ask", "feel free to ask", "if you have any questions", "if you have questions", "let me know if you have any questions", "please let me know", "if you need anything else", "let me know if you need anything else", "don't hesitate to ask"),
		remove("i hope this helps", "hope this helps", "hope it helps", "i hope this answers your question", "hope this answers your question", "i hope this was helpful", "i hope that helps"),
		remove("as you can see", "as you can see below", "as can be seen", "as shown below", "as shown above", "as you can see above"),
		remove("as i mentioned earlier", "as mentioned earlier", "as i mentioned before", "as mentioned before", "as i said", "as noted above", "as noted earlier", "as previously mentioned", "as mentioned above"),
		remove("let's go step by step", "let's do this step by step", "let's break it down", "let me walk you through", "let me explain", "let me show you", "let me give you", "let's take a look"),
		remove("sure, here is", "certainly, here is", "no problem, here", "happy to help, here", "sure, let's", "here is the", "here is a", "here are", "here is", "here's"),
		remove("certainly", "absolutely", "of course", "no problem", "you're welcome", "you are welcome", "happy to help", "glad to help", "great question", "excellent question", "good question"),
		remove("i would suggest", "i suggest", "i would recommend", "i recommend", "my suggestion is"),
		remove("in my opinion", "i believe", "from my perspective", "in my experience"),
		remove("if you run into any issues", "if you encounter any issues", "if you have any trouble", "if you need further assistance", "if you need any help"),
		remove("you might also want to", "you may want to", "you might want to"),
		remove("now the code runs smoothly and without errors", "the code runs smoothly"),

		// --- Russian: structuring and explanatory patterns (replaced) ---
		replace("стоит учесть, что", "важно отметить, что", "следует отметить, что", "необходимо отметить, что", "стоит отметить, что", "важно помнить, что", "стоит помнить, что", "обратите внимание, что", "обратите внимание на то, что"),
		replace("для применения", "для того чтобы применить это, вам нужно", "для этого вам нужно", "чтобы это сделать, вам нужно"),
		replace("этот блок выполняет", "данный фрагмент кода отвечает за", "данный блок отвечает за", "этот фрагмент кода отвечает за"),
		replace("логика работы", "этот код делает следующее", "данный код выполняет следующее"),
		replace("итого", "в заключение, можно сказать, что", "в заключение", "подводя итог", "подведем итог", "подведём итог", "резюмируя", "резюмируя вышесказанное", "итак, подведем итог", "в целом, можно сказать, что"),
		replace("в результате", "таким образом, этот код", "в итоге мы получаем", "в итоге", "в результате", "в конечном счете", "в конечном счёте", "следовательно", "таким образом"),

		// --- English: structuring and explanatory patterns (replaced) ---
		replace("note that", "it is important to note that", "please keep in mind that", "it is worth mentioning that", "it's worth noting that", "it is worth noting that", "please note that"),
		replace("to do this", "in order to achieve this, you need to", "in order to do this", "in order to achieve this", "to achieve this"),
		replace("this block handles", "this specific snippet is responsible for", "this code is responsible for", "this block is responsible for"),
		replace("summary", "in conclusion", "to conclude", "to sum up", "to summarize", "in summary", "summing up", "in short", "in a nutshell", "all in all", "overall"),
		replace("so", "thus, this code", "as a result, we get", "therefore", "hence", "as a result"),

		// --- Standalone leftovers (wiped out last, after longer variants) ---
		remove("вот", "итак"),
	}
)

func supportedExt(ext string) bool {
	switch ext {
	case ".c", ".cpp", ".h", ".py", ".go", ".html", ".css", ".js", ".md", ".txt":
		return true
	}
	return false
}

func main() {
	dirPath := flag.String("d", "", "Path to the directory to scan")
	filePath := flag.String("f", "", "Path to a single file to process")
	flag.Parse()

	if *dirPath == "" && *filePath == "" {
		fmt.Println("Error: specify a directory via the -d flag or a single file via the -f flag")
		os.Exit(1)
	}

	rand.Seed(time.Now().UnixNano())

	if *filePath != "" {
		if _, err := os.Stat(*filePath); err != nil {
			fmt.Printf("Error: cannot access file %s: %v\n", *filePath, err)
			os.Exit(1)
		}
		ext := strings.ToLower(filepath.Ext(*filePath))
		if !supportedExt(ext) {
			fmt.Printf("Error: unsupported file extension %q\n", ext)
			os.Exit(1)
		}
		processFile(*filePath, ext)
		fmt.Println("Processing completed successfully.")
		return
	}

	err := filepath.WalkDir(*dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip internal system directories like .git, .idea, .vscode.
		// Never skip the root itself: when -d is "." or ".\" its name is ".",
		// which would otherwise abort the whole walk before any file is seen.
		if d.IsDir() && path != *dirPath && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}

		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if supportedExt(ext) {
			processFile(path, ext)
		}

		return nil
	})

	if err != nil {
		fmt.Printf("Error processing directory: %v\n", err)
	} else {
		fmt.Println("Processing completed successfully.")
	}
}

// cleanText applies the text-level cleaning rules (emoji, pseudographics,
// quote normalization, spacing, AI-phrase substitution) to a chunk of text.
func cleanText(line []byte, ext string) []byte {
	cleaned := line

	// 2. Strip emojis
	cleaned = emojiRegex.ReplaceAll(cleaned, []byte(""))

	// 3. Replace dash variants (box-drawing "─", en dash "–", em dash "—",
	// horizontal bar "―") with "-"
	cleaned = dashRegex.ReplaceAll(cleaned, []byte("-"))

	// 4. Convert French quotes « and » to < and >
	cleaned = bytes.ReplaceAll(cleaned, []byte("«"), []byte("<"))
	cleaned = bytes.ReplaceAll(cleaned, []byte("»"), []byte(">"))

	// 5. Convert English directional smart quotes “ and ” to standard straight quotes "
	cleaned = bytes.ReplaceAll(cleaned, []byte("“"), []byte("\""))
	cleaned = bytes.ReplaceAll(cleaned, []byte("”"), []byte("\""))

	// 6. Remove excessive spaces before punctuation marks
	cleaned = spacesBeforePunctuation.ReplaceAll(cleaned, []byte("$1"))

	// 7. Convert asterisk lists to hyphen lists (Isolated to .md and .txt files)
	if ext == ".md" || ext == ".txt" {
		cleaned = markdownListRegex.ReplaceAll(cleaned, []byte("$1- "))
	}

	// 8. Substitute AI-specific patterns
	for _, pr := range aiPhrases {
		cleaned = pr.Pattern.ReplaceAll(cleaned, pr.Replacement)
	}

	return cleaned
}

// cState carries multi-line lexer state for C/C++/Go/JS/CSS sources.
type cState struct {
	inBlock bool // inside a /* ... */ block comment
	inRaw   bool // inside a ` ... ` raw/template string
}

// cleanCLine applies cleanText only to the comment portions of a C/C++/Go/JS/CSS
// line, leaving code and string/char/raw literals byte-for-byte intact. st
// carries the multi-line state (block comments and raw strings) across lines.
// lineComment controls whether // starts a line comment (true for C/C++/Go/JS,
// false for CSS where // only appears inside URLs).
func cleanCLine(line []byte, st *cState, ext string, lineComment bool) []byte {
	var out []byte
	var commentBuf []byte
	var rawBuf []byte
	inComment := st.inBlock
	inRaw := st.inRaw
	inStr := false
	inChr := false

	i := 0
	n := len(line)
	for i < n {
		c := line[i]

		if inComment {
			commentBuf = append(commentBuf, c)
			if c == '*' && i+1 < n && line[i+1] == '/' {
				commentBuf = append(commentBuf, '/')
				i += 2
				inComment = false
				out = append(out, cleanText(commentBuf, ext)...)
				commentBuf = nil
			} else {
				i++
			}
			continue
		}

		if inRaw {
			rawBuf = append(rawBuf, c)
			if c == '\\' && i+1 < n && line[i+1] == '`' {
				rawBuf = append(rawBuf, line[i+1])
				i += 2
				continue
			}
			if c == '`' {
				inRaw = false
				out = append(out, rawBuf...)
				rawBuf = nil
				i++
			} else {
				i++
			}
			continue
		}

		if inStr || inChr {
			out = append(out, c)
			if c == '\\' && i+1 < n {
				out = append(out, line[i+1])
				i += 2
				continue
			}
			if (inStr && c == '"') || (inChr && c == '\'') {
				inStr = false
				inChr = false
			}
			i++
			continue
		}

		if c == '/' && i+1 < n && line[i+1] == '*' {
			commentBuf = append(commentBuf, '/', '*')
			i += 2
			inComment = true
			continue
		}
		if lineComment && c == '/' && i+1 < n && line[i+1] == '/' {
			commentBuf = append(commentBuf, line[i:]...)
			out = append(out, cleanText(commentBuf, ext)...)
			commentBuf = nil
			i = n
			continue
		}
		if c == '`' {
			rawBuf = append(rawBuf, c)
			i++
			inRaw = true
			continue
		}
		if c == '"' {
			out = append(out, c)
			i++
			inStr = true
			continue
		}
		if c == '\'' {
			out = append(out, c)
			i++
			inChr = true
			continue
		}

		out = append(out, c)
		i++
	}

	st.inBlock = inComment
	st.inRaw = inRaw
	if inComment {
		out = append(out, cleanText(commentBuf, ext)...)
	}
	if inRaw {
		out = append(out, rawBuf...)
	}
	return out
}

// pyState carries multi-line lexer state for Python sources (triple-quoted
// strings / docstrings that span lines).
type pyState struct {
	inTriple    bool
	tripleDelim byte // '"' or '\''
}

// cleanPyLine applies cleanText only to the # comment portions of a Python line,
// leaving code and string literals (including triple-quoted docstrings) intact.
func cleanPyLine(line []byte, st *pyState, ext string) []byte {
	var out []byte
	var tripleBuf []byte
	inTriple := st.inTriple
	tripleDelim := st.tripleDelim
	inStr := false
	strDelim := byte(0)

	i := 0
	n := len(line)
	for i < n {
		c := line[i]

		if inTriple {
			tripleBuf = append(tripleBuf, c)
			if c == tripleDelim && i+2 < n && line[i+1] == tripleDelim && line[i+2] == tripleDelim {
				tripleBuf = append(tripleBuf, line[i+1], line[i+2])
				i += 3
				inTriple = false
				out = append(out, tripleBuf...)
				tripleBuf = nil
			} else {
				i++
			}
			continue
		}

		if inStr {
			out = append(out, c)
			if c == '\\' && i+1 < n {
				out = append(out, line[i+1])
				i += 2
				continue
			}
			if c == strDelim {
				inStr = false
			}
			i++
			continue
		}

		if (c == '"' || c == '\'') && i+2 < n && line[i+1] == c && line[i+2] == c {
			tripleBuf = append(tripleBuf, c, line[i+1], line[i+2])
			i += 3
			inTriple = true
			tripleDelim = c
			continue
		}
		if c == '#' {
			out = append(out, cleanText(line[i:], ext)...)
			i = n
			continue
		}
		if c == '"' || c == '\'' {
			out = append(out, c)
			i++
			inStr = true
			strDelim = c
			continue
		}

		out = append(out, c)
		i++
	}

	st.inTriple = inTriple
	st.tripleDelim = tripleDelim
	if inTriple {
		out = append(out, tripleBuf...)
	}
	return out
}

// htmlState carries multi-line lexer state for HTML (comments and tags that
// span lines) plus the depth of nested <a>/<form>/<header> elements whose
// content must be left untouched.
type htmlState struct {
	inComment  bool
	inTag      bool
	skipDepth  int
	commentBuf []byte
	tagBuf     []byte
	textBuf    []byte
}

// cleanHTMLLine applies cleanText to HTML comments and to text nodes, but leaves
// tags and the content of <a>, <form> and <header> elements byte-for-byte intact.
func cleanHTMLLine(line []byte, st *htmlState, ext string) []byte {
	var out []byte
	i := 0
	n := len(line)
	for i < n {
		c := line[i]

		if st.inComment {
			st.commentBuf = append(st.commentBuf, c)
			if c == '>' && len(st.commentBuf) >= 3 &&
				st.commentBuf[len(st.commentBuf)-2] == '-' && st.commentBuf[len(st.commentBuf)-3] == '-' {
				st.inComment = false
				if st.skipDepth == 0 {
					out = append(out, cleanText(st.commentBuf, ext)...)
				} else {
					out = append(out, st.commentBuf...)
				}
				st.commentBuf = nil
				i++
			} else {
				i++
			}
			continue
		}

		if st.inTag {
			st.tagBuf = append(st.tagBuf, c)
			if c == '>' {
				st.inTag = false
				updateSkipDepth(st, st.tagBuf)
				out = append(out, st.tagBuf...)
				st.tagBuf = nil
				i++
			} else {
				i++
			}
			continue
		}

		if c == '<' {
			flushHTMLText(st, &out, ext)
			if i+3 < n && line[i+1] == '!' && line[i+2] == '-' && line[i+3] == '-' {
				st.inComment = true
				st.commentBuf = append([]byte{}, line[i:i+4]...)
				i += 4
			} else {
				st.inTag = true
				st.tagBuf = append([]byte{}, c)
				i++
			}
			continue
		}

		st.textBuf = append(st.textBuf, c)
		i++
	}

	flushHTMLText(st, &out, ext)
	return out
}

// flushHTMLText emits the accumulated text node, cleaning it only when not
// inside a skipped <a>/<form>/<header> element.
func flushHTMLText(st *htmlState, out *[]byte, ext string) {
	if len(st.textBuf) == 0 {
		return
	}
	if st.skipDepth == 0 {
		*out = append(*out, cleanText(st.textBuf, ext)...)
	} else {
		*out = append(*out, st.textBuf...)
	}
	st.textBuf = nil
}

// updateSkipDepth adjusts the skip depth when a tag closes, tracking nested
// <a>, <form> and <header> elements.
func updateSkipDepth(st *htmlState, tag []byte) {
	if len(tag) == 0 || tag[0] != '<' {
		return
	}
	t := tag[1:]
	closing := false
	if len(t) > 0 && t[0] == '/' {
		closing = true
		t = t[1:]
	}
	var name []byte
	for _, ch := range t {
		if ch == ' ' || ch == '>' || ch == '/' || ch == '\t' || ch == '\n' || ch == '\r' {
			break
		}
		name = append(name, ch)
	}
	switch strings.ToLower(string(name)) {
	case "a", "form", "header":
		if closing {
			if st.skipDepth > 0 {
				st.skipDepth--
			}
		} else {
			st.skipDepth++
		}
	}
}

func processFile(filePath string, ext string) {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		fmt.Printf("Failed to read metadata for %s: %v\n", filePath, err)
		return
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Printf("Failed to read file %s: %v\n", filePath, err)
		return
	}

	// 1. Standardize line endings to LF (\n) and split into lines
	standardized := bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	lines := bytes.Split(standardized, []byte("\n"))

	var modifiedLines [][]byte
	fileWasChanged := false

	var cSt cState
	var pySt pyState
	var htmlSt htmlState
	isCSource := ext == ".c" || ext == ".cpp" || ext == ".h" || ext == ".go" || ext == ".js"
	isCSS := ext == ".css"
	isPy := ext == ".py"
	isHTML := ext == ".html"

	// Process file line by line to accurately track line numbers and changes
	for i, originalLine := range lines {
		// Skip processing for the last empty element if file ends with a trailing newline
		if i == len(lines)-1 && len(originalLine) == 0 {
			modifiedLines = append(modifiedLines, originalLine)
			continue
		}

		var cleaned []byte
		switch {
		case isCSource:
			// C/C++/Go/JS: clean only comment text; leave code and string/char/raw
			// literals byte-for-byte intact.
			cleaned = cleanCLine(originalLine, &cSt, ext, true)
		case isCSS:
			// CSS: clean only /* ... */ comments; // is not a comment here.
			cleaned = cleanCLine(originalLine, &cSt, ext, false)
		case isPy:
			// Python: clean only # comments; leave code and strings/docstrings intact.
			cleaned = cleanPyLine(originalLine, &pySt, ext)
		case isHTML:
			// HTML: clean comments and text nodes; leave tags and the content of
			// <a>/<form>/<header> elements intact.
			cleaned = cleanHTMLLine(originalLine, &htmlSt, ext)
		default:
			// .md/.txt and anything else: clean the whole line.
			cleaned = cleanText(originalLine, ext)
		}

		// Check if this specific line has changed
		if !bytes.Equal(originalLine, cleaned) {
			fileWasChanged = true
			// Format output strictly requested by user
			fmt.Printf("%s:\n\tLine %d\n\tOriginal: %s\n\tModified: %s\n", filePath, i+1, string(originalLine), string(cleaned))
		}

		modifiedLines = append(modifiedLines, cleaned)
	}

	// Join processed lines back into a unified byte buffer
	finalContent := bytes.Join(modifiedLines, []byte("\n"))

	// 9. Collapse multiple empty lines across the global content layout.
	// Applied only to plain-text files (.md/.txt); for code/HTML sources the
	// layout is left untouched so that only comment/text content is altered.
	var globalCleaned []byte
	if ext == ".md" || ext == ".txt" {
		globalCleaned = consecutiveNewlines.ReplaceAll(finalContent, []byte("\n\n"))
	} else {
		globalCleaned = finalContent
	}

	if !bytes.Equal(finalContent, globalCleaned) {
		fileWasChanged = true
	}

	// If no modifications were made at all, do not rewrite the file on disk
	if !fileWasChanged && bytes.Equal(content, globalCleaned) {
		return
	}

	backupPath := filePath + ".bak"
	if err := os.WriteFile(backupPath, content, fileInfo.Mode()); err != nil {
		fmt.Printf("Failed to create backup %s: %v\n", backupPath, err)
		return
	}
	if err := os.Chtimes(backupPath, fileInfo.ModTime(), fileInfo.ModTime()); err != nil {
		fmt.Printf("Failed to update timestamps for %s: %v\n", backupPath, err)
		return
	}

	err = os.WriteFile(filePath, globalCleaned, fileInfo.Mode())
	if err != nil {
		fmt.Printf("Failed to write file %s: %v\n", filePath, err)
		return
	}

	// 10. Obfuscate file timestamps (original time minus 0 to 359 random minutes)
	randomMinutes := rand.Intn(360)
	newMtime := fileInfo.ModTime().Add(-time.Duration(randomMinutes) * time.Minute)

	err = os.Chtimes(filePath, fileInfo.ModTime(), newMtime)
	if err != nil {
		fmt.Printf("Failed to update timestamps for %s: %v\n", filePath, err)
		return
	}
}