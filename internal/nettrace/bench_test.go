package nettrace

import (
	"testing"
)

// Our benchmark loop is similar to the one from golang.org/x/net/trace, so we
// can compare results.
func runBench(b *testing.B, events int) {
	nTraces := (b.N + events + 1) / events

	for i := 0; i < nTraces; i++ {
		tr := New("bench", "test")
		for j := 0; j < events; j++ {
			tr.Printf("%d", j)
		}
		tr.Finish()
	}
}

func BenchmarkTrace_2(b *testing.B) {
	runBench(b, 2)
}

func BenchmarkTrace_10(b *testing.B) {
	runBench(b, 10)
}

func BenchmarkTrace_100(b *testing.B) {
	runBench(b, 100)
}

func BenchmarkTrace_1000(b *testing.B) {
	runBench(b, 1000)
}

func BenchmarkTrace_10000(b *testing.B) {
	runBench(b, 10000)
}

func BenchmarkNewAndFinish(b *testing.B) {
	for i := 0; i < b.N; i++ {
		tr := New("bench", "test")
		tr.Finish()
	}
}

func BenchmarkPrintf(b *testing.B) {
	tr := New("bench", "test")
	defer tr.Finish()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Keep this without any formatting, so we measure our code instead of
		// the performance of fmt.Sprintf.
		tr.Printf("this is printf")
	}
}
