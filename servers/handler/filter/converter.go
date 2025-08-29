package filter

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	variablePattern         = regexp.MustCompile(`\{([^}]+)}`)
	spacesPattern           = regexp.MustCompile(`\s+`)
	outerParenthesesPattern = regexp.MustCompile(`^\(\s*(.+)\s*\)$`)
)

// goToPythonFilterConverter converts Go-style filters to Python-style filters
type goToPythonFilterConverter struct {
	// Regex patterns for different filter components
	patterns map[string]*regexp.Regexp
}

// newGoToPythonFilterConverter creates a new converter instance
func newGoToPythonFilterConverter() *goToPythonFilterConverter {
	return &goToPythonFilterConverter{
		patterns: map[string]*regexp.Regexp{
			// variables wrapped in {variable}
			"variables": variablePattern,
			// multiple spaces
			"spaces": spacesPattern,
			// outer parentheses detection
			"outer_parens": outerParenthesesPattern,
		},
	}
}

// convert transforms a Go-style filter to Python-style filter
func (c *goToPythonFilterConverter) convert(goFilter string) string {
	// Step 1: Normalize spaces
	result := c.patterns["spaces"].ReplaceAllString(strings.TrimSpace(goFilter), " ")

	// Step 2: Remove outer parentheses wrapper if it wraps the entire expression
	result = c.removeOuterParentheses(result)

	// Step 3: convert {variable} back to variable
	result = c.patterns["variables"].ReplaceAllString(result, "$1")

	// Step 4: Handle 'in' operator with quoted arrays
	result = c.handleInArrays(result)

	// Step 5: Remove parentheses around individual conditions
	result = c.removeConditionParentheses(result)

	// Step 6: Clean up spacing
	result = c.patterns["spaces"].ReplaceAllString(result, " ")
	result = strings.TrimSpace(result)

	return result
}

// removeOuterParentheses removes the outermost parentheses if they wrap the entire expression
func (c *goToPythonFilterConverter) removeOuterParentheses(input string) string {
	input = strings.TrimSpace(input)

	// Check if the entire expression is wrapped in parentheses
	if strings.HasPrefix(input, "(") && strings.HasSuffix(input, ")") {
		// Count parentheses to ensure we're removing the outer wrapper
		openCount := 0
		for i, char := range input {
			if char == '(' {
				openCount++
			} else if char == ')' {
				openCount--
				// If we reach 0 before the end, these aren't outer wrapper parentheses
				if openCount == 0 && i < len(input)-1 {
					return input
				}
			}
		}
		// If we made it here, the parentheses wrap the whole expression
		if openCount == 0 {
			return input[1 : len(input)-1]
		}
	}

	return input
}

// handleInArrays processes 'in' operator with quoted arrays
func (c *goToPythonFilterConverter) handleInArrays(input string) string {
	inPattern := regexp.MustCompile(`(\w+)\s+in\s+\[([^]]+)]`)
	return inPattern.ReplaceAllStringFunc(input, func(match string) string {
		parts := inPattern.FindStringSubmatch(match)
		if len(parts) == 3 {
			variable := parts[1]
			arrayContent := parts[2]

			// Split array elements and process each
			elements := strings.Split(arrayContent, ",")

			var processedElements []string
			for _, element := range elements {
				element = strings.TrimSpace(element)

				// Handle hex values - remove 0x prefix for method_id
				if strings.Contains(variable, "method_id") && strings.HasPrefix(element, "0x") {
					element = element[2:]
				}

				processedElements = append(processedElements, element)
			}

			return fmt.Sprintf("%s in [%s]", variable, strings.Join(processedElements, ", "))
		}
		return match
	})
}

// removeConditionParentheses removes parentheses around individual conditions
func (c *goToPythonFilterConverter) removeConditionParentheses(input string) string {
	// Split by logical operators while preserving them
	logicalPattern := regexp.MustCompile(`\s+(and|or)\s+`)
	parts := logicalPattern.Split(input, -1)
	operators := logicalPattern.FindAllString(input, -1)

	// Process each part
	var result strings.Builder
	for i, part := range parts {
		part = strings.TrimSpace(part)

		// Remove outer parentheses from individual conditions
		part = c.removeSimpleParentheses(part)

		result.WriteString(part)

		// Add logical operator if not the last part
		if i < len(operators) {
			result.WriteString(operators[i])
		}
	}

	return result.String()
}

// removeSimpleParentheses removes parentheses from simple conditions
func (c *goToPythonFilterConverter) removeSimpleParentheses(condition string) string {
	condition = strings.TrimSpace(condition)

	// Handle nested parentheses by counting
	if strings.HasPrefix(condition, "(") && strings.HasSuffix(condition, ")") {
		// Count parentheses to see if we can safely remove outer ones
		openCount := 0
		canRemove := true

		for i, char := range condition {
			if char == '(' {
				openCount++
			} else if char == ')' {
				openCount--
				// If we hit 0 before the end, we can't remove outer parentheses
				if openCount == 0 && i < len(condition)-1 {
					canRemove = false
					break
				}
			}
		}

		if canRemove && openCount == 0 {
			inner := condition[1 : len(condition)-1]
			// Recursively remove if there are more layers
			return c.removeSimpleParentheses(inner)
		}
	}

	return condition
}
