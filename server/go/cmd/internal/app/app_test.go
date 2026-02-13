package app

import "testing"

func TestRuntimeBaseURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "explicit localhost", in: "127.0.0.1:8080", want: "http://127.0.0.1:8080"},
		{name: "bind all v4", in: "0.0.0.0:8080", want: "http://127.0.0.1:8080"},
		{name: "bind all v6", in: "[::]:9090", want: "http://127.0.0.1:9090"},
		{name: "ipv6 host", in: "[2001:db8::1]:9090", want: "http://[2001:db8::1]:9090"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := runtimeBaseURL(tc.in)
			if got != tc.want {
				t.Fatalf("runtimeBaseURL(%q)=%q want=%q", tc.in, got, tc.want)
			}
		})
	}
}

func TestWSBaseURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{in: "http://127.0.0.1:8080", want: "ws://127.0.0.1:8080"},
		{in: "https://arc.example.com", want: "wss://arc.example.com"},
		{in: "127.0.0.1:8080", want: "ws://127.0.0.1:8080"},
	}

	for _, tc := range cases {
		got := wsBaseURL(tc.in)
		if got != tc.want {
			t.Fatalf("wsBaseURL(%q)=%q want=%q", tc.in, got, tc.want)
		}
	}
}
