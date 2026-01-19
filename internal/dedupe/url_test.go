package dedupe

import "testing"

func TestCanonicalizeURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		out  string
	}{
		{
			name: "trim and lowercase host",
			in:   " HTTPS://WWW.Example.COM/News ",
			out:  "https://example.com/News",
		},
		{
			name: "drop tracking params",
			in:   "https://example.com/post?utm_source=twitter&ref=home&id=123",
			out:  "https://example.com/post?id=123",
		},
		{
			name: "strip fragment and default port",
			in:   "http://example.com:80/path#section",
			out:  "https://example.com/path",
		},
		{
			name: "clean path",
			in:   "https://example.com/a/b/../c/",
			out:  "https://example.com/a/c",
		},
		{
			name: "fallback scheme",
			in:   "example.com/item?utm_medium=email",
			out:  "https://example.com/item",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanonicalizeURL(tc.in); got != tc.out {
				t.Fatalf("expected %q, got %q", tc.out, got)
			}
		})
	}
}
