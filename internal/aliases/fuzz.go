// Fuzz testing for package aliases.

//go:build gofuzz
// +build gofuzz

package aliases

import "bytes"

func Fuzz(data []byte) int {
	interesting := 0
	aliases, _ := parseReader("domain", bytes.NewReader(data))

	// Mark cases with actual aliases as more interesting.
	for _, rcpts := range aliases {
		if len(rcpts) > 0 {
			interesting = 1
			break
		}
	}

	return interesting
}
