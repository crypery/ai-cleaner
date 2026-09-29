# AI-Trace Cleaner

> [Russian version](README_RU.md)

`AI-Trace Cleaner` is a Go-based command-line utility designed to automatically sanitize source code (`.c`, `.cpp`, `.h`, `.py`, `.go`, `.html`, `.css`, `.js`) and documentation (`.md`) files from distinct artifacts, structural patterns, and stylistic traces left by Large Language Models (LLMs) during text generation.

The tool helps bring your AI-generated assets into a natural, "human-written" look, mitigating both automated detection and manual code-review suspicions.

## ✨ Features

The utility recursively scans the specified directory and applies a comprehensive cleaning pipeline to target files:

1. **Emoji Stripping:** Completely removes any Unicode emojis, which AI models frequently abuse to structure documentation or markdown sections.
2. **Pseudographics Correction:** Replaces long box-drawing symbols (`─`) with standard ASCII hyphens (`-`).
3. **Quote Standardization:** 
   * Replaces French guillemets (`«` and `»`) with angle brackets (`<` and `>`) respectively.
   * Replaces English directional "smart" curly quotes (`“` and `”`) with standard straight double quotes (`"`).
4. **Punctuation Normalization:** Automatically eliminates stray whitespace before punctuation marks (`.`, `,`, `:`, `;`, `!`, `?`) often found in AI outputs.
5. **List Formatting (Strictly `.md`):** Detects bullet lists formatted with asterisks (`* `) and converts them to standard hyphens (`- `) while carefully preserving the original indentation levels and nested list hierarchy.
6. **AI Stop-Phrase Suppression:** Suppresses over 20 characteristic introductory, transition, and concluding phrases in both English and Russian (e.g., *“Sure, here is...”*, *“It is important to note that...”*, *“In conclusion...”*), replacing them with professional technical synonyms or wiping them entirely.
7. **Consecutive Newline Collapsing:** Automatically detects excessive vertical spacing (3 or more consecutive newline characters) and collapses them down to a single blank line.
8. **Line Ending Standardization:** Enforces the Unix standard LF (`\n`) line endings across all files by removing Windows CR (`\r`) carriage returns.
9. **Anti-Forensics Timestamp Obfuscation:** To prevent giving away a mass-editing pattern where all files feature identical edit times, the tool reads each file's original modification time (`mtime`) and individually offsets it backward by a randomized duration (ranging from 0 to 359 minutes).

## 🚀 Installation

To build this tool from source, you need [Go](https://go.dev) (version 1.16 or higher) installed on your system.

1. Clone this repository or save the source code as `ai-cleaner.go`.
2. Compile the executable binary:
   ```bash
   go build -o ai-cleaner ai-cleaner.go
   ```

## 💻 Usage

Run the utility via your terminal. You must pass the target directory path using the required `-dir` flag.

### Run Directly via Go:
```bash
go run ai-cleaner.go -dir="/path/to/your/project"
```

### Run the Compiled Binary:
* **Linux / macOS:**
  ```bash
  ./ai-cleaner -dir="/path/to/your/project"
  ```
* **Windows:**
  ```cmd
  ai-cleaner.exe -dir="C:\(\path\to\your\project\)"
  ```

## ⚠️ Important Safety Considerations

* **C/C++ Pointer Safety:** The asterisk-to-hyphen list conversion logic (`* ` to `- `) is isolated and executed **strictly within files with the `.md` extension**. This ensures that pointer dereferencing, type declarations like `int*`, and multi-line comments `/* ... */` in C/C++ source code remain completely unharmed.
* **Smart Rewriting:** The tool only rewrites a file back to the disk if unwanted AI patterns were actually found and changed. If a file is already clean, it won't be touched, preserving its authentic system metadata.

## 🤝 Tips for Ultimate Obfuscation

Automated text rewriting handles only part of the detection footprint. For the best stealth results, consider the following:
* Run your C/C++ code through a code-formatting tool like `clang-format` before running this utility to dilute the "perfectly uniform" indentation rhythm characteristic of AI models.
* Avoid committing all modified files in a single, massive Git push. Break your changes into 2-3 logical commits to simulate organic human updates.

## License

GNU AGPL v3 (Affero GPL)