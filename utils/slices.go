package utils

// Exists - checks if a field exists in slice
func Exists(field string, slice []string) bool {
	for _, valid := range slice {
		if field == valid {
			return true
		}
	}
	return false
}
