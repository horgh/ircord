package irc

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitMessage(t *testing.T) {
	tests := []struct {
		input  string
		sz     int
		output []string
	}{
		{"foo", 0, []string{"foo"}},
		{"foo", 1, []string{"f", "o", "o"}},
		{"foo", 2, []string{"fo", "o"}},
		{"foo", 3, []string{"foo"}},
		{"foo", 4, []string{"foo"}},
		{"foo", 5, []string{"foo"}},
	}

	for _, test := range tests {
		t.Run(
			fmt.Sprintf("%s with size %d", test.input, test.sz),
			func(t *testing.T) {
				pieces := splitMessage(test.input, test.sz)
				assert.Equal(t, test.output, pieces, "split into correct pieces")
			},
		)
	}
}
