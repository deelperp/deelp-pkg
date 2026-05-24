package auth

import "testing"

func TestMascararToken(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "<vazio>"},
		{"Bearer ", "<vazio>"},
		{"abc", "<curto:3>"},
		{"abcdefghij12", "<curto:12>"},
		{"Bearer eyJhbGciOiJIUzI1NiJ9.payload.signxx", "eyJhbG…(35)…gnxx"},
		{"abcdef1234567890wxyz", "abcdef…(20)…wxyz"},
	}
	for _, c := range cases {
		got := MascararToken(c.in)
		if got != c.want {
			t.Errorf("MascararToken(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
