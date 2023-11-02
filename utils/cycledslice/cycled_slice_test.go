package cycledslice

import (
	"testing"
)

func TestCycledSliceInt(t *testing.T) {
	intSlice := []int{1, 2, 3, 4, 5}
	cycledIntSlice := NewCycledSlice(intSlice)

	expected := []int{1, 2, 3, 4, 5, 1, 2, 3, 4, 5}

	for i, expectedValue := range expected {
		actualValue := cycledIntSlice.Next()
		if actualValue != expectedValue {
			t.Errorf("Test failed at index %d: expected %d, got %d", i, expectedValue, actualValue)
		}
	}
}

func TestCycledSliceString(t *testing.T) {
	stringSlice := []string{"apple", "banana", "cherry"}
	cycledStringSlice := NewCycledSlice(stringSlice)

	expected := []string{"apple", "banana", "cherry", "apple", "banana", "cherry"}

	for i, expectedValue := range expected {
		actualValue := cycledStringSlice.Next()
		if actualValue != expectedValue {
			t.Errorf("Test failed at index %d: expected %s, got %s", i, expectedValue, actualValue)
		}
	}
}

func TestCycledSliceEmpty(t *testing.T) {
	emptySlice := []int{}
	cycledEmptySlice := NewCycledSlice(emptySlice)

	// When the slice is empty, Next() should return the zero value for int, which is 0.
	for i := 0; i < 5; i++ {
		actualValue := cycledEmptySlice.Next()
		if actualValue != 0 {
			t.Errorf("Test failed at index %d: expected 0, got %d", i, actualValue)
		}
	}
}

func TestCurrentInt(t *testing.T) {
	intSlice := []int{1, 2, 3, 4, 5}
	cycledIntSlice := NewCycledSlice(intSlice)

	expected := []int{1, 2, 3, 4, 5, 1, 2, 3, 4, 5}

	for i, expectedValue := range expected {
		currentValue := cycledIntSlice.Current()
		if currentValue != expectedValue {
			t.Errorf("Test failed at index %d: expected %d, got %d", i, expectedValue, currentValue)
		}

		_ = cycledIntSlice.Next() // Advance to the next element
	}
}

func TestCurrentString(t *testing.T) {
	stringSlice := []string{"apple", "banana", "cherry"}
	cycledStringSlice := NewCycledSlice(stringSlice)

	expected := []string{"apple", "banana", "cherry", "apple", "banana", "cherry"}

	for i, expectedValue := range expected {
		currentValue := cycledStringSlice.Current()
		if currentValue != expectedValue {
			t.Errorf("Test failed at index %d: expected %s, got %s", i, expectedValue, currentValue)
		}

		_ = cycledStringSlice.Next() // Advance to the next element
	}
}

func TestCurrentEmpty(t *testing.T) {
	emptySlice := []interface{}{}
	cycledEmptySlice := NewCycledSlice(emptySlice)

	// When the slice is empty, Current() should return the zero value for int, which is 0.
	for i := 0; i < 5; i++ {
		currentValue := cycledEmptySlice.Current()
		if currentValue != nil {
			t.Errorf("Test failed at index %d: expected nil, got %v", i, currentValue)
		}

		_ = cycledEmptySlice.Next() // Advance to the next element (even though it's empty)
	}
}
