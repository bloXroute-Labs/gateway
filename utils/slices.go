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
