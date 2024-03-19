package main

import (
	"math/rand"
	"strings"
)

var chars = []rune("abcdefghijklmnopqrstuvwxyz")

func generateId() string {
	out := strings.Builder{}
	for i := 0; i < 5; i++ {
		idx := rand.Int() % len(chars)
		out.WriteRune(chars[idx])
	}
	return out.String()
}

func generatePort(start, end int) int {
	offset := end - start
	return start + (rand.Int() % offset)
}
