package webasis

import "testing"

func TestToken(t *testing.T) {
	token := ToToken("hello", "world")
	name, secret := FromToken(token)
	if name != "hello" {
		t.Error(name)
	}
	if secret != "world" {
		t.Error(secret)
	}
}

func Benchmark_ToToken(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ToToken("hello0000000000000000000000", "wo0000000000000000000000000000rld")
	}
}
func Benchmark_FromToken(b *testing.B) {
	token := ToToken("hello0000000000000000000000", "wo0000000000000000000000000000rld")
	for i := 0; i < b.N; i++ {
		FromToken(token)
	}
}
