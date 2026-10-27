package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// step is one turn of a simulated game.
type step struct {
	guess      Word
	pattern    Pattern
	candidates int // candidates left before this guess
	probe      bool
}

// game is the full record of the solver playing against one secret.
type game struct {
	secret Word
	steps  []step
	solved bool
	left   []Word // candidates still possible when the game ended unsolved
}

// play runs the solver against `secret` for up to 6 turns, recording every
// guess so a failure can be explained afterwards.
func play(dict []Word, secret Word) game {
	g := game{secret: secret}
	candidates := dict
	for turn := 1; turn <= 6; turn++ {
		guess := chooseGuess(candidates, dict, 7-turn)
		pattern := computePattern(guess, secret)
		g.steps = append(g.steps, step{
			guess:      guess,
			pattern:    pattern,
			candidates: len(candidates),
			probe:      !contains(candidates, guess),
		})
		if guess == secret {
			g.solved = true
			return g
		}
		candidates = filterCandidates(candidates, guess, pattern)
	}
	g.left = candidates
	return g
}

// runReport plays every dictionary word as the secret and writes the words
// not found within 6 guesses, with the full guess history of each, to path.
func runReport(dict []Word, path string) {
	var failures []game
	for _, secret := range dict {
		if g := play(dict, secret); !g.solved {
			failures = append(failures, g)
		}
	}

	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "не удалось создать %s: %v\n", path, err)
		os.Exit(1)
	}
	w := bufio.NewWriter(f)

	fmt.Fprintf(w, "Стратегия: %s\n", strategy)
	fmt.Fprintf(w, "Слов в словаре: %d\n", len(dict))
	fmt.Fprintf(w, "Не найдено за 6 попыток: %d\n", len(failures))

	for _, g := range failures {
		fmt.Fprintf(w, "\n=== %s\n", wordToStr(g.secret))
		for i, s := range g.steps {
			mark := ""
			if s.probe {
				mark = "  [разведчик]"
			}
			fmt.Fprintf(w, "  %d. %s -> %s  (кандидатов до хода: %d)%s\n",
				i+1, wordToStr(s.guess), s.pattern, s.candidates, mark)
		}
		left := make([]string, len(g.left))
		for i, c := range g.left {
			left[i] = wordToStr(c)
		}
		fmt.Fprintf(w, "  Осталось кандидатов: %d: %s\n", len(left), strings.Join(left, ", "))
	}

	if err := w.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "ошибка записи в %s: %v\n", path, err)
		os.Exit(1)
	}
	if err := f.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "ошибка записи в %s: %v\n", path, err)
		os.Exit(1)
	}
	fmt.Printf("Не найдено за 6 попыток: %d из %d. Отчёт: %s\n", len(failures), len(dict), path)
}
