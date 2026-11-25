package toolkit

import "strings"

// DiffOp is one line of a diff: Op is " " (common), "-" (only in A) or
// "+" (only in B).
type DiffOp struct {
	Op   string `json:"op"`
	Text string `json:"text"`
}

// DiffLines computes a line-level diff between a and b using a longest common
// subsequence, the same model behind Burp's Comparer.
func DiffLines(a, b string) []DiffOp {
	la := strings.Split(a, "\n")
	lb := strings.Split(b, "\n")
	n, m := len(la), len(lb)

	// LCS length table.
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if la[i] == lb[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	var out []DiffOp
	i, j := 0, 0
	for i < n && j < m {
		if la[i] == lb[j] {
			out = append(out, DiffOp{" ", la[i]})
			i++
			j++
		} else if lcs[i+1][j] >= lcs[i][j+1] {
			out = append(out, DiffOp{"-", la[i]})
			i++
		} else {
			out = append(out, DiffOp{"+", lb[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, DiffOp{"-", la[i]})
	}
	for ; j < m; j++ {
		out = append(out, DiffOp{"+", lb[j]})
	}
	return out
}
