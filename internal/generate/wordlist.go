package generate

import (
	_ "embed"
	"strings"
)

// WordlistSize is the number of words in the embedded EFF long wordlist. Each
// word therefore contributes log2(7776) ≈ 12.925 bits of entropy to a
// passphrase. Exposed so callers can compute and display passphrase entropy.
const WordlistSize = 7776

//go:embed eff_large_wordlist.txt
var effLargeWordlist string

// words is the parsed EFF long wordlist. The source file is tab-separated
// "<dice>\t<word>" lines (e.g. "11111\tabacus"); only the word is retained.
var words = parseWordlist(effLargeWordlist)

func parseWordlist(raw string) []string {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Keep the field after the tab; fall back to the whole line if absent.
		if i := strings.IndexByte(line, '\t'); i >= 0 {
			line = line[i+1:]
		}
		out = append(out, line)
	}
	return out
}
