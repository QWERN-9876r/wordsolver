// wordsolver is a solver for a Wordle-style "5 letters" game played with
// 5-letter Russian nouns (nominative singular).
//
// The core idea is information-theoretic: on every turn, instead of just
// trying to guess the secret word directly, the solver picks the guess
// that splits the remaining candidates as evenly as possible across the
// possible feedback patterns (Shannon entropy). That squeezes the most
// information out of every single guess, which is what lets a small,
// fixed number of guesses (6) reliably pin down the answer.
//
// The default strategy ("value") is a cheaper heuristic: each letter is
// worth the number of remaining candidates containing it, a word is worth
// the sum of its distinct letters, and the solver suggests the most
// valuable word that still fits all feedback so far. The entropy strategy
// is still available via -strategy=entropy.
//
// Feedback encoding per letter, matching the game's rules:
//
//	2 = right letter, right place  ("на месте")
//	1 = right letter, wrong place  ("есть, но не на этом месте")
//	0 = letter not in the word     ("нет")
//
// Usage:
//
//	go run .                 interactive mode: solver suggests guesses,
//	                          you type the feedback you got from the game
//	go run . -secret=комод    automatic demo against a known secret
//	go run . -report=failures.txt
//	                          play every dictionary word and write the ones
//	                          not found in 6 guesses, with their guesses,
//	                          to failures.txt (see report.go)
//	go run . -selftest        run the solver against every dictionary word
//	                          and report whether 6 guesses always suffice
package main

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
)

//go:embed words.txt
var wordlistFile string

// Word is a fixed-size 5-letter word. Arrays (unlike slices) are
// comparable, so Word works directly as a map key and with ==.
type Word [5]rune

// Pattern is the per-letter feedback for one guess: 0, 1 or 2 per slot.
type Pattern [5]uint8

func (p Pattern) String() string {
	var b strings.Builder
	for _, v := range p {
		b.WriteByte('0' + byte(v))
	}
	return b.String()
}

func toWord(s string) (Word, bool) {
	r := []rune(s)
	if len(r) != 5 {
		return Word{}, false
	}
	var w Word
	copy(w[:], r)
	return w, true
}

func loadDictionary() []Word {
	lines := strings.Split(strings.TrimSpace(wordlistFile), "\n")
	words := make([]Word, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		w, ok := toWord(line)
		if !ok {
			continue
		}
		words = append(words, w)
	}
	sort.Slice(words, func(i, j int) bool {
		return string(words[i][:]) < string(words[j][:])
	})
	return words
}

// computePattern compares guess against secret the way the game does,
// handling repeated letters correctly: a letter is only marked "present"
// as many times as it actually still occurs in the secret after exact
// matches are accounted for.
func computePattern(guess, secret Word) Pattern {
	var result Pattern
	remaining := map[rune]int{}

	for i := 0; i < 5; i++ {
		if guess[i] == secret[i] {
			result[i] = 2
		} else {
			remaining[secret[i]]++
		}
	}
	for i := 0; i < 5; i++ {
		if result[i] == 2 {
			continue
		}
		if remaining[guess[i]] > 0 {
			result[i] = 1
			remaining[guess[i]]--
		} else {
			result[i] = 0
		}
	}
	return result
}

func patternKey(p Pattern) int {
	key := 0
	for _, v := range p {
		key = key*3 + int(v)
	}
	return key
}

// entropy returns the expected information (in bits) gained by guessing
// `guess` against the current candidate set. It buckets the candidates by
// the feedback pattern `guess` would produce against each of them, then
// computes the Shannon entropy of that bucket distribution. A guess that
// splits the candidates into many small, even buckets scores high; a
// guess that leaves most candidates in one big bucket scores low.
func entropy(guess Word, candidates []Word) float64 {
	buckets := map[int]int{}
	for _, cand := range candidates {
		buckets[patternKey(computePattern(guess, cand))]++
	}
	n := float64(len(candidates))
	h := 0.0
	for _, count := range buckets {
		p := float64(count) / n
		h -= p * math.Log2(p)
	}
	return h
}

// bestGuess picks the guess (from `allowed`) that maximizes entropy
// against `candidates`. Ties are broken in favor of a guess that is
// itself still a possible secret, so a lucky exact match can end the
// game early instead of "wasting" a turn on a pure information probe.
func bestGuess(candidates, allowed []Word) Word {
	candidateSet := make(map[Word]bool, len(candidates))
	for _, c := range candidates {
		candidateSet[c] = true
	}

	best := allowed[0]
	bestH := -1.0
	bestIsCandidate := false

	for _, g := range allowed {
		h := entropy(g, candidates)
		isCand := candidateSet[g]
		switch {
		case h > bestH+1e-9:
			best, bestH, bestIsCandidate = g, h, isCand
		case math.Abs(h-bestH) <= 1e-9 && isCand && !bestIsCandidate:
			best, bestH, bestIsCandidate = g, h, isCand
		}
	}
	return best
}

// letterValues computes the "value" of every letter: the number of words in
// `words` that contain it at least once. Letters common among the remaining
// candidates are worth testing, because their feedback splits the set.
func letterValues(words []Word) map[rune]int {
	values := map[rune]int{}
	for _, w := range words {
		seen := map[rune]bool{}
		for _, r := range w {
			if !seen[r] {
				seen[r] = true
				values[r]++
			}
		}
	}
	return values
}

// wordValue sums the values of the distinct letters of w. A repeated letter
// is counted once: a second copy tells us almost nothing new.
func wordValue(w Word, values map[rune]int) int {
	seen := map[rune]bool{}
	total := 0
	for _, r := range w {
		if !seen[r] {
			seen[r] = true
			total += values[r]
		}
	}
	return total
}

// splitValues is letterValues over the candidates, keeping only letters that
// tell them apart. A letter found in every candidate is worth 0: testing it
// cannot split the set. (For ranking candidates themselves this changes
// nothing, since such a letter adds the same amount to every candidate.)
func splitValues(candidates []Word) map[rune]int {
	values := letterValues(candidates)
	for r, v := range values {
		if v == len(candidates) {
			delete(values, r)
		}
	}
	return values
}

// mostValuable picks the highest-value word among `words`. Among equally
// valuable words it prefers the one whose feedback splits `candidates` into
// the most groups: letter values can't tell that, say, ж/м/х separate
// жатка/матка/хатка while ч/п/с/ы do not.
func mostValuable(words, candidates []Word, values map[rune]int) (Word, int) {
	bestV := -1
	var top []Word
	for _, w := range words {
		v := wordValue(w, values)
		if v > bestV {
			bestV, top = v, top[:0]
		}
		if v == bestV {
			top = append(top, w)
		}
	}
	best, bestG := top[0], 0
	if len(top) > 1 {
		for _, w := range top {
			if g := countGroups(w, candidates); g > bestG {
				best, bestG = w, g
			}
		}
	}
	return best, bestV
}

// countGroups returns how many distinct feedback patterns guess would get
// across the candidates, i.e. into how many groups it splits them.
func countGroups(guess Word, candidates []Word) int {
	seen := map[Pattern]bool{}
	for _, c := range candidates {
		seen[computePattern(guess, c)] = true
	}
	return len(seen)
}

// valueGuess suggests the most valuable word that still fits the feedback,
// with letter values recomputed over the current candidates. When there are
// more candidates than turns left, it may instead suggest a "probe": a word
// from the whole dictionary that is known not to be the answer but tests
// more of the distinguishing letters at once. That breaks traps like
// гр?за/грива/груда/грыжа, where guessing candidates one by one only
// eliminates one word per turn.
func valueGuess(candidates, dict []Word, turnsLeft int) Word {
	values := splitValues(candidates)
	best, bestV := mostValuable(candidates, candidates, values)
	if len(candidates) <= turnsLeft || turnsLeft == 1 {
		return best // enough turns to try them all, or a probe can no longer win
	}
	if probe, probeV := mostValuable(dict, candidates, values); probeV > bestV {
		return probe
	}
	return best
}

// strategy selects how the next guess is chosen; set by the -strategy flag.
var strategy = "value"

func chooseGuess(candidates, dict []Word, turnsLeft int) Word {
	if strategy == "entropy" {
		return bestGuess(candidates, dict)
	}
	return valueGuess(candidates, dict, turnsLeft)
}

func contains(words []Word, w Word) bool {
	for _, c := range words {
		if c == w {
			return true
		}
	}
	return false
}

func filterCandidates(candidates []Word, guess Word, pattern Pattern) []Word {
	out := candidates[:0:0]
	for _, c := range candidates {
		if computePattern(guess, c) == pattern {
			out = append(out, c)
		}
	}
	return out
}

func wordToStr(w Word) string { return string(w[:]) }

// ---- interactive mode -----------------------------------------------

func runInteractive(dict []Word) {
	candidates := dict
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Решаю игру «5 букв». На каждом шаге буду предлагать слово;")
	fmt.Println("введите результат как 5 цифр: 2=на месте, 1=есть но не там, 0=нет.")
	fmt.Println("Например: 20110")
	fmt.Println()

	for turn := 1; turn <= 6; turn++ {
		if len(candidates) == 0 {
			fmt.Println("Подходящих слов не осталось — проверьте, не было ли ошибки в вводе.")
			return
		}
		if len(candidates) == 1 {
			fmt.Printf("Ответ: %s\n", wordToStr(candidates[0]))
			return
		}

		guess := chooseGuess(candidates, dict, 7-turn)
		fmt.Printf("Попытка %d/6, осталось кандидатов: %d\n", turn, len(candidates))
		fmt.Printf("Предлагаю слово: %s\n", wordToStr(guess))
		if !contains(candidates, guess) {
			fmt.Println("(это слово-разведчик: ответом оно быть не может, но проверит больше букв)")
		}
		fmt.Print("Своё слово (Enter = согласиться): ")

		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		actual := guess
		if line != "" {
			if w, ok := toWord(line); ok {
				actual = w
			} else {
				fmt.Println("Слово должно быть из 5 букв, использую предложенное.")
			}
		}

		var pattern Pattern
		for {
			fmt.Print("Результат (пример 20110): ")
			resp, _ := reader.ReadString('\n')
			resp = strings.TrimSpace(resp)
			if len(resp) == 5 && isValidPatternString(resp) {
				for i, ch := range resp {
					pattern[i] = uint8(ch - '0')
				}
				break
			}
			fmt.Println("Нужно ровно 5 символов из {0,1,2}.")
		}

		if pattern == (Pattern{2, 2, 2, 2, 2}) {
			fmt.Printf("Угадано: %s (попытка %d)\n", wordToStr(actual), turn)
			return
		}
		candidates = filterCandidates(candidates, actual, pattern)
	}
	fmt.Println("Лимит в 6 попыток исчерпан.")
}

func isValidPatternString(s string) bool {
	for _, ch := range s {
		if ch < '0' || ch > '2' {
			return false
		}
	}
	return true
}

// ---- automatic demo against a known secret ---------------------------

func runAuto(dict []Word, secretStr string) {
	secret, ok := toWord(secretStr)
	if !ok {
		fmt.Fprintf(os.Stderr, "секрет должен быть словом из 5 букв: %q\n", secretStr)
		os.Exit(1)
	}
	candidates := dict
	for turn := 1; turn <= 6; turn++ {
		if len(candidates) == 1 {
			candidates = []Word{candidates[0]}
		}
		guess := chooseGuess(candidates, dict, 7-turn)
		pattern := computePattern(guess, secret)
		probe := ""
		if !contains(candidates, guess) {
			probe = "  [разведчик]"
		}
		fmt.Printf("Попытка %d: %s -> %s  (осталось кандидатов: %d)%s\n",
			turn, wordToStr(guess), pattern.String(), len(candidates), probe)
		if guess == secret {
			fmt.Printf("Угадано за %d попыток(ки): %s\n", turn, wordToStr(secret))
			return
		}
		candidates = filterCandidates(candidates, guess, pattern)
	}
	fmt.Println("Не угадано за 6 попыток (см. ниже про ограничения словаря).")
}

// ---- self-test: verify 6 guesses suffice for every dictionary word ---

func runSelfTest(dict []Word) {
	counts := map[int]int{}
	var failures []Word
	maxGuesses := 0

	for _, secret := range dict {
		candidates := dict
		solved := false
		for turn := 1; turn <= 6; turn++ {
			guess := chooseGuess(candidates, dict, 7-turn)
			pattern := computePattern(guess, secret)
			if guess == secret {
				counts[turn]++
				if turn > maxGuesses {
					maxGuesses = turn
				}
				solved = true
				break
			}
			candidates = filterCandidates(candidates, guess, pattern)
		}
		if !solved {
			failures = append(failures, secret)
		}
	}

	fmt.Printf("Словарь: %d слов\n\n", len(dict))
	total := 0
	sum := 0
	for turn := 1; turn <= 6; turn++ {
		if c, ok := counts[turn]; ok {
			fmt.Printf("  за %d попытку(и): %d слов\n", turn, c)
			total += c
			sum += turn * c
		}
	}
	if total > 0 {
		fmt.Printf("\nСреднее число попыток: %.2f\n", float64(sum)/float64(total))
	}
	fmt.Printf("Максимум попыток на одно слово: %d\n", maxGuesses)
	if len(failures) > 0 {
		fmt.Printf("\nНЕ решено за 6 попыток (%d слов):\n", len(failures))
		for _, w := range failures {
			fmt.Println(" ", wordToStr(w))
		}
	} else {
		fmt.Println("\nВсе слова словаря угадываются не более чем за 6 попыток.")
	}
}

func main() {
	secret := flag.String("secret", "", "прогнать автоматическую демонстрацию против этого слова")
	selftest := flag.Bool("selftest", false, "проверить весь словарь: хватает ли 6 попыток на каждое слово")
	report := flag.String("report", "", "прогнать весь словарь и записать не найденные за 6 попыток слова в этот файл")
	suggest := flag.Bool("suggest", false, "вернуть следующую подсказку в формате JSON")
	history := flag.String("history", "", "история ходов для -suggest в формате слово:результат,...")
	flag.StringVar(&strategy, "strategy", "value", "выбор слова: value (самое ценное из подходящих) или entropy")
	flag.Parse()
	if strategy != "value" && strategy != "entropy" {
		fmt.Fprintf(os.Stderr, "неизвестная стратегия %q: нужно value или entropy\n", strategy)
		os.Exit(1)
	}

	dict := loadDictionary()
	if *suggest {
		if err := runSuggestion(dict, *history); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	switch {
	case *report != "":
		runReport(dict, *report)
	case *selftest:
		runSelfTest(dict)
	case *secret != "":
		runAuto(dict, *secret)
	default:
		runInteractive(dict)
	}
}

type suggestionResponse struct {
	Guess      string `json:"guess"`
	Candidates int    `json:"candidates"`
}

func runSuggestion(dict []Word, history string) error {
	candidates := dict
	turns := []string{}
	if history != "" {
		turns = strings.Split(history, ",")
	}
	if len(turns) >= 6 {
		return fmt.Errorf("лимит в 6 попыток исчерпан")
	}
	for _, turn := range turns {
		parts := strings.Split(turn, ":")
		if len(parts) != 2 {
			return fmt.Errorf("неверный ход %q: ожидалось слово:результат", turn)
		}
		guess, ok := toWord(parts[0])
		if !ok {
			return fmt.Errorf("неверное слово в ходе %q: нужно ровно 5 букв", turn)
		}
		if len(parts[1]) != 5 || !isValidPatternString(parts[1]) {
			return fmt.Errorf("неверный результат в ходе %q: нужны 5 цифр из {0,1,2}", turn)
		}
		var pattern Pattern
		for i, ch := range parts[1] {
			pattern[i] = uint8(ch - '0')
		}
		candidates = filterCandidates(candidates, guess, pattern)
	}
	if len(candidates) == 0 {
		return fmt.Errorf("подходящих слов не осталось — проверьте результаты ходов")
	}

	guess := chooseGuess(candidates, dict, 6-len(turns))
	return json.NewEncoder(os.Stdout).Encode(suggestionResponse{
		Guess:      wordToStr(guess),
		Candidates: len(candidates),
	})
}
