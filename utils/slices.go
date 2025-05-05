package utils

// Exists - checks if a field exists in slice
func Exists[K comparable](field K, slice []K) bool {
	for _, valid := range slice {
		if field == valid {
			return true
		}
	}
	return false
}

// Filter - filters a slice based on a predicate
func Filter[K any](slice []K, f func(K) bool) []K {
	var filtered []K
	for _, element := range slice {
		if f(element) {
			filtered = append(filtered, element)
		}
	}
	return filtered
}

// ConvertSlice - converts a slice of one type to another
func ConvertSlice[I, J any](s []I, converter func(I) J) []J {
	var e []J
	for _, item := range s {
		e = append(e, converter(item))
	}
	return e
}

// CompareLists compares two lists and returns the unique elements in each list
func CompareLists[K comparable](l1 []K, l2 []K) ([]K, []K) {
	l1Set := make(map[K]struct{})
	for _, el := range l1 {
		l1Set[el] = struct{}{}
	}

	l2Set := make(map[K]struct{})
	for _, el := range l2 {
		l2Set[el] = struct{}{}
	}

	l1Uniq := make([]K, 0)
	for _, el := range l1 {
		if _, ok := l2Set[el]; !ok {
			l1Uniq = append(l1Uniq, el)
		}
	}

	l2Uniq := make([]K, 0)
	for _, el := range l2 {
		if _, ok := l1Set[el]; !ok {
			l2Uniq = append(l2Uniq, el)
		}
	}

	return l1Uniq, l2Uniq
}
