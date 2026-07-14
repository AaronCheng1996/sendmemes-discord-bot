package images

// naturalLess reports whether a should sort before b using a case-insensitive
// natural ordering. Runs of digits are compared by numeric value, so that, for
// example, "2.jpg" sorts before "10.jpg" and "a2" before "a10". Non-digit bytes
// are compared on their lowercased form. When one string is a prefix of the
// other, the shorter one sorts first.
func naturalLess(a, b string) bool {
	return naturalCompare(a, b) < 0
}

// naturalCompare returns -1, 0, or 1 comparing a and b with natural ordering.
func naturalCompare(a, b string) int {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		ai, bj := a[i], b[j]
		if isDigit(ai) && isDigit(bj) {
			if c := compareDigitRuns(a, b, &i, &j); c != 0 {
				return c
			}
			continue
		}
		la, lb := lowerByte(ai), lowerByte(bj)
		if la != lb {
			if la < lb {
				return -1
			}
			return 1
		}
		i++
		j++
	}
	switch {
	case i < len(a):
		return 1
	case j < len(b):
		return -1
	default:
		return 0
	}
}

// compareDigitRuns compares the digit runs starting at a[*i] and b[*j],
// advancing *i and *j past those runs. It returns -1/0/1; a 0 result means the
// runs are numerically equal (ties broken by leading-zero count for stability).
func compareDigitRuns(a, b string, i, j *int) int {
	// Skip leading zeros, remembering how many each run had.
	startA, startB := *i, *j
	for *i < len(a) && a[*i] == '0' {
		*i++
	}
	for *j < len(b) && b[*j] == '0' {
		*j++
	}
	// Locate the end of each (zero-stripped) digit run.
	endA, endB := *i, *j
	for endA < len(a) && isDigit(a[endA]) {
		endA++
	}
	for endB < len(b) && isDigit(b[endB]) {
		endB++
	}

	lenA, lenB := endA-*i, endB-*j
	result := 0
	switch {
	case lenA != lenB:
		if lenA < lenB {
			result = -1
		} else {
			result = 1
		}
	default:
		for k := 0; k < lenA; k++ {
			if a[*i+k] != b[*j+k] {
				if a[*i+k] < b[*j+k] {
					result = -1
				} else {
					result = 1
				}
				break
			}
		}
	}

	*i, *j = endA, endB
	if result != 0 {
		return result
	}
	// Numerically equal: fewer leading zeros sorts first for a deterministic order.
	lzA, lzB := (endA-lenA)-startA, (endB-lenB)-startB
	switch {
	case lzA < lzB:
		return -1
	case lzA > lzB:
		return 1
	default:
		return 0
	}
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func lowerByte(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}
