package sqlite

import (
	"testing"
)

func TestParseCircle(t *testing.T) {
	cases := []struct {
		in string
		ok bool
	}{
		{"<(5,1),3>", true},
		{"((1,2),100)", true},
		{" 1 , 3 , 5 ", true},
		{" ( ( 1 , 2 ) , 3 ) ", true},
		{" ( 100 , 200 ) , 10 ", true},
		{" < ( 100 , 1 ) , 115 > ", true},
		{"<(3,5),0>", true},
		{"<(3,5),NaN>", true},
		{"<(-100,0),-100>", false},
		{"<(100,200),10", false},
		{"<(100,200),10> x", false},
		{"1abc,3,5", false},
		{"(3,(1,2),3)", false},
	}
	for _, tc := range cases {
		x, y, r, ok := parseCircle(tc.in)
		if ok != tc.ok {
			t.Errorf("parseCircle(%q) = (x=%g, y=%g, r=%g, ok=%v), want %v", tc.in, x, y, r, ok, tc.ok)
		}
	}
}
