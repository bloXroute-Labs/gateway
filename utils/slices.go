package utils

import (
	"errors"
	"fmt"
)

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

// RemoveItemFromList is a helper function for remove a tx of a tx slice
func RemoveItemFromList(itemList []string, item string) ([]string, error) {
	if len(itemList) == 0 {
		return itemList, errors.New("empty list given")
	} else if len(itemList) == 1 && item != itemList[0] {
		return itemList, fmt.Errorf("item %v does not exist", item)
	}

	for i, t := range itemList {
		if t == item {
			itemList[i] = itemList[len(itemList)-1]
			return itemList[:len(itemList)-1], nil
		}
	}

	return itemList, fmt.Errorf("item %v does not exist", item)
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
