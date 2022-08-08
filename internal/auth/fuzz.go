// Fuzz testing for package aliases.

//go:build gofuzz
// +build gofuzz

package auth

func Fuzz(data []byte) int {
	//	user, domain, passwd, err := DecodeResponse(string(data))
	interesting := 0
	_, _, _, err := DecodeResponse(string(data))
	if err == nil {
		interesting = 1
	}

	return interesting
}
