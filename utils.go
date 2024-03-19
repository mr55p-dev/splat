package main

import (
	"math/rand"
	"strings"
	"sync"
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

type Counter struct {
	sync.Mutex
	count int
}

func (c *Counter) next() int {
	c.Lock()
	defer c.Unlock()
	out := c.count
	c.count++
	return out
}

func NewCounter(start int) *Counter {
	return &Counter{count: start}
}
