package automate

import "testing"

func TestParseTemplate(t *testing.T) {
	tm := parseTemplate("/user/§123§/profile", "§")
	if tm.holes() != 1 {
		t.Fatalf("holes=%d want 1", tm.holes())
	}
	if tm.bases[0] != "123" {
		t.Fatalf("base=%q want 123", tm.bases[0])
	}
	if got := tm.render([]string{"999"}); got != "/user/999/profile" {
		t.Fatalf("render=%q", got)
	}
	// Two positions.
	tm2 := parseTemplate("a=§1§&b=§2§", "§")
	if tm2.holes() != 2 {
		t.Fatalf("holes=%d want 2", tm2.holes())
	}
	if got := tm2.render([]string{"x", "y"}); got != "a=x&b=y" {
		t.Fatalf("render=%q", got)
	}
}

func TestSniper(t *testing.T) {
	// 2 positions, set of 3 -> 6 requests, one position changed at a time.
	v, err := buildVectors("sniper", 2, []string{"A", "B"}, [][]string{{"1", "2", "3"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 6 {
		t.Fatalf("got %d vectors want 6", len(v))
	}
	// First three change position 0, keep base at position 1.
	if v[0][0] != "1" || v[0][1] != "B" {
		t.Fatalf("v0=%v", v[0])
	}
	if v[3][0] != "A" || v[3][1] != "1" {
		t.Fatalf("v3=%v", v[3])
	}
}

func TestBatteringRam(t *testing.T) {
	v, _ := buildVectors("batteringram", 2, []string{"A", "B"}, [][]string{{"x", "y"}})
	if len(v) != 2 {
		t.Fatalf("got %d want 2", len(v))
	}
	if v[0][0] != "x" || v[0][1] != "x" {
		t.Fatalf("both positions should be x: %v", v[0])
	}
}

func TestPitchfork(t *testing.T) {
	v, err := buildVectors("pitchfork", 2, []string{"A", "B"}, [][]string{{"1", "2", "3"}, {"a", "b"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 2 { // min length
		t.Fatalf("got %d want 2", len(v))
	}
	if v[1][0] != "2" || v[1][1] != "b" {
		t.Fatalf("v1=%v", v[1])
	}
}

func TestClusterBomb(t *testing.T) {
	v, err := buildVectors("clusterbomb", 2, []string{"A", "B"}, [][]string{{"1", "2"}, {"a", "b", "c"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 6 { // 2 * 3
		t.Fatalf("got %d want 6", len(v))
	}
}

func TestExpandNumbers(t *testing.T) {
	out, err := expand(PayloadSet{Type: "numbers", From: 1, To: 5, Step: 2})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1", "3", "5"}
	if len(out) != len(want) {
		t.Fatalf("got %v", out)
	}
	for i := range want {
		if out[i] != want[i] {
			t.Fatalf("got %v want %v", out, want)
		}
	}
}

func TestExpandBrute(t *testing.T) {
	out, _ := expand(PayloadSet{Type: "brute", Charset: "ab", Min: 1, Max: 2})
	// length 1: a,b ; length 2: aa,ab,ba,bb = 6 total
	if len(out) != 6 {
		t.Fatalf("got %d (%v) want 6", len(out), out)
	}
}
