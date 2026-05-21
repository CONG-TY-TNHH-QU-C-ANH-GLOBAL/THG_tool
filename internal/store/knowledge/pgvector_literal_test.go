package knowledge

import "testing"

// pgVectorLiteral: format must be parseable by pgvector. The text
// representation is "[1.0,2.0,3.0]" — comma-separated, square-bracket-wrapped.
func TestPGVectorLiteral(t *testing.T) {
	cases := []struct {
		in   []float32
		want string
	}{
		{[]float32{}, "[]"},
		{[]float32{1.0}, "[1]"},
		{[]float32{1.0, 2.0, 3.0}, "[1,2,3]"},
		{[]float32{0.5, -0.25}, "[0.5,-0.25]"},
	}
	for _, c := range cases {
		if got := pgVectorLiteral(c.in); got != c.want {
			t.Errorf("pgVectorLiteral(%v) = %q; want %q", c.in, got, c.want)
		}
	}
}
