package courier

import "testing"

func TestReceiveURL(t *testing.T) {
	tests := []struct {
		base string
		want string
	}{
		{"https://courier.example.com", "https://courier.example.com/c/tph/receive"},
		{"https://courier.example.com/", "https://courier.example.com/c/tph/receive"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := ReceiveURL(tc.base); got != tc.want {
			t.Fatalf("ReceiveURL(%q) = %q, want %q", tc.base, got, tc.want)
		}
	}
}
