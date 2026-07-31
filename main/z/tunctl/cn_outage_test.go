package tunctl

import "testing"

func TestDomesticOutageByCounts(t *testing.T) {
	cases := []struct {
		ok, n int
		want  bool
	}{
		{3, 3, false},
		{2, 3, false},
		{1, 3, true},
		{0, 3, true},
		{1, 2, false},
		{0, 2, true},
	}
	for _, tc := range cases {
		if got := domesticOutageByCounts(tc.ok, tc.n); got != tc.want {
			t.Fatalf("ok=%d n=%d: got %v want %v", tc.ok, tc.n, got, tc.want)
		}
	}
}
