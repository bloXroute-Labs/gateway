package filter

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/vm"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"

	"github.com/bloXroute-Labs/gateway/v2/utils"
)

// maxDepth is the maximum depth of nested expressions allowed in the filter
const maxDepth = 10

var (
	defaultEnv = map[string]interface{}{
		"gas":                      float64(0),
		"gas_price":                float64(0),
		"value":                    float64(0),
		"to":                       "0x0",
		"from":                     "",
		"method_id":                "",
		"type":                     "",
		"chain_id":                 0,
		"max_fee_per_gas":          0,
		"max_priority_fee_per_gas": 0,
		"max_fee_per_blob_gas":     0,
	}

	equalityOperatorReplacer = regexp.MustCompile(`([^=!<>])=([^=])`)
	quoteSetter              = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)\s*([=!><]=?|in)\s*(\[[^]]+]|[^\s)(]+)`)

	// ErrFromFieldIncludable is returned when the 'from' filter is not allowed due to flag 'txFromFieldIncludable' being false
	ErrFromFieldIncludable = fmt.Errorf("the 'from' filter is not allowed in this context, please remove it from the filters")

	// ErrEmptyFields is returned when the 'fields' map is empty during evaluation
	ErrEmptyFields = fmt.Errorf("fields cannot be empty, please provide at least one field to evaluate the expression")
)

// Expression represents a compiled filter expression that can be evaluated against provided fields.
type Expression struct {
	program   *vm.Program
	rawFilter string
	args      []string
}

// NewDefaultExpression creates a new Expression with default env.
func NewDefaultExpression(filters string, txFromFieldIncludable bool) (*Expression, error) {
	return newExpression(filters, txFromFieldIncludable, defaultEnv)
}

// NewExpression creates a new Expression with the provided env map.
func NewExpression(filters string, txFromFieldIncludable bool, env map[string]interface{}) (*Expression, error) {
	return newExpression(filters, txFromFieldIncludable, env)
}

// NewExpression creates and validates filters from a user's request
func newExpression(filters string, txFromFieldIncludable bool, env map[string]interface{}) (*Expression, error) {
	program, err := parseFilter(filters, env)
	if err != nil {
		return nil, fmt.Errorf("error parsing Filters: %v", err)
	}

	// check if it is allowed to use 'from' filter
	if !txFromFieldIncludable && utils.Exists("from", program.Constants) {
		return nil, ErrFromFieldIncludable
	}

	log.Infof("filters: %v", filters)

	var args []string
	for _, arg := range program.Constants {
		strArg, ok := arg.(string)
		if !ok {
			continue
		}
		_, ok = env[strArg]
		if ok {
			args = append(args, strArg)
		}
	}

	return &Expression{
		program:   program,
		rawFilter: filters,
		args:      args,
	}, nil
}

// Evaluate evaluates the expression against the provided fields.
func (e *Expression) Evaluate(fields map[string]interface{}) (bool, error) {
	if e.program == nil {
		return false, fmt.Errorf("program is not compiled")
	}

	if len(fields) == 0 {
		return false, ErrEmptyFields
	}

	// evaluate the expression with the provided transaction data
	result, err := expr.Run(e.program, fields)
	if err != nil {
		return false, fmt.Errorf("error evaluating expression: %w", err)
	}

	// convert the result to boolean
	boolResult, ok := result.(bool)
	if !ok {
		return false, fmt.Errorf("expected boolean result, got %T", result)
	}

	return boolResult, nil
}

// Args returns the arguments used in the expression.
func (e *Expression) Args() []string {
	return e.args
}

// String returns the string representation of the expression.
func (e *Expression) String() string {
	if e.rawFilter == "" {
		return "n/a"
	}

	return e.rawFilter
}

func parseFilter(filters string, env map[string]interface{}) (*vm.Program, error) {
	// lowercase all the input
	filters = strings.ToLower(filters)

	// if the filter values are go-type filters, e.g.: {value}, convert it to general expression filters
	if strings.Contains(filters, "{") {
		filters = newGoToPythonFilterConverter().convert(filters)
	} else {
		// replace the equality operator
		filters = equalityOperatorReplacer.ReplaceAllString(filters, "$1==$2")

		// wrap all non-numeric values in quotes and add '0x' at the beginning if the value does not start with it
		filters = quoteSetter.ReplaceAllStringFunc(filters, func(m string) string {
			parts := quoteSetter.FindStringSubmatch(m)
			if len(parts) != 4 {
				return m
			}

			variable := parts[1]
			operator := parts[2]
			value := parts[3]

			// handle array values (for 'in' operator)
			if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
				return handleArrayValue(variable, operator, value)
			}

			// handle single values
			return handleSingleValue(variable, operator, value)
		})
	}

	// compile the expression
	program, err := expr.Compile(filters, expr.Env(env))
	if err != nil {
		return nil, fmt.Errorf("failed to compile expression: %w", err)
	}

	// semantic validation
	if err = validateExpressionSemanticsRecursive(program.Node(), 1, true); err != nil {
		return nil, fmt.Errorf("semantic validation failed: %w", err)
	}

	// validate the program for gas-related filters
	err = validateGasRelatedFilters(program)
	if err != nil {
		return nil, fmt.Errorf("failed to validate program: %w", err)
	}

	return program, nil
}

func validateExpressionSemanticsRecursive(node ast.Node, depth int, isTopLevel bool) error {
	if depth > maxDepth {
		return fmt.Errorf("expression is too deeply nested (max depth: %d)", maxDepth)
	}

	switch n := node.(type) {
	case *ast.IdentifierNode:
		if isTopLevel {
			// bare identifiers like "from" are invalid
			return fmt.Errorf("bare identifier '%s' is not allowed", n.Value)
		}

	case *ast.BinaryNode:
		if err := validateExpressionSemanticsRecursive(n.Left, depth+1, false); err != nil {
			return err
		}
		if err := validateExpressionSemanticsRecursive(n.Right, depth+1, false); err != nil {
			return err
		}

		// reject `in []`
		if n.Operator == "in" {
			if arrayNode, ok := n.Right.(*ast.ArrayNode); ok && len(arrayNode.Nodes) == 0 {
				return fmt.Errorf("empty array on right-hand side of 'in' is not allowed")
			}
		}

	case *ast.UnaryNode:
		return validateExpressionSemanticsRecursive(n.Node, depth+1, false)

	case *ast.CallNode:
		for _, arg := range n.Arguments {
			if err := validateExpressionSemanticsRecursive(arg, depth+1, false); err != nil {
				return err
			}
		}

	case *ast.ArrayNode:
		for _, elem := range n.Nodes {
			if err := validateExpressionSemanticsRecursive(elem, depth+1, false); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateGasRelatedFilters(program *vm.Program) error {
	clauses := extractClauses(program.Node())
	for _, c := range clauses {
		if err := validateClause(c); err != nil {
			return err
		}
	}
	return nil
}

type clause struct {
	typeValue *string
	filters   map[string]bool
}

func extractClauses(node ast.Node) []*clause {
	var clauses []*clause
	extract(node, &clauses, 1)
	return clauses
}

func extract(node ast.Node, clauses *[]*clause, depth int) {
	if depth > maxDepth {
		return
	}

	switch n := node.(type) {
	case *ast.BinaryNode:
		switch n.Operator {
		case "||":
			extract(n.Left, clauses, depth+1)
			extract(n.Right, clauses, depth+1)
		case "&&":
			// group both sides into one clause
			c := &clause{filters: make(map[string]bool)}
			collectClauseFilters(n.Left, c)
			collectClauseFilters(n.Right, c)
			*clauses = append(*clauses, c)
		default:
			// simple binary expression like x > 1
			c := &clause{filters: make(map[string]bool)}
			collectClauseFilters(n, c)
			*clauses = append(*clauses, c)
		}
	default:
		c := &clause{filters: make(map[string]bool)}
		collectClauseFilters(n, c)
		*clauses = append(*clauses, c)
	}
}

func collectClauseFilters(node ast.Node, c *clause) {
	switch n := node.(type) {
	case *ast.BinaryNode:
		switch n.Operator {
		case "&&":
			collectClauseFilters(n.Left, c)
			collectClauseFilters(n.Right, c)
		case "==":
			if id, ok := n.Left.(*ast.IdentifierNode); ok && id.Value == "type" {
				if strVal, ok := n.Right.(*ast.StringNode); ok {
					c.typeValue = &strVal.Value
				}
			}
		default:
			if id, ok := n.Left.(*ast.IdentifierNode); ok {
				c.filters[id.Value] = true
			}
		}
	case *ast.IdentifierNode:
		c.filters[n.Value] = true
	case *ast.CallNode:
		for _, arg := range n.Arguments {
			collectClauseFilters(arg, c)
		}
	}
}

func validateClause(c *clause) error {
	if c.typeValue == nil {
		// No type => no restrictions now
		return nil
	}

	t := *c.typeValue
	f := c.filters

	switch t {
	case "0", "0x00", "1", "0x01":
		if f["max_fee_per_gas"] || f["max_priority_fee_per_gas"] || f["max_fee_per_blob_gas"] {
			return fmt.Errorf("type == %s cannot be used with max_fee_per_gas, max_priority_fee_per_gas, or max_fee_per_blob_gas", t)
		}
	case "2", "0x02":
		if f["gas_price"] || f["max_fee_per_blob_gas"] {
			return fmt.Errorf("type == %s cannot be used with gas_price or max_fee_per_blob_gas", t)
		}
	case "3", "0x03":
		if f["gas_price"] || f["max_fee_per_gas"] || f["max_priority_fee_per_gas"] {
			return fmt.Errorf("type == %s cannot be used with gas_price, max_fee_per_gas, or max_priority_fee_per_gas", t)
		}
	case "4", "0x04":
		if f["gas_price"] || f["max_fee_per_blob_gas"] {
			return fmt.Errorf("type == %s cannot be used with gas_price or max_fee_per_blob_gas", t)
		}
	}

	return nil
}

func handleArrayValue(variable, operator, arrayValue string) string {
	// extract array content (remove [ and ])
	arrayContent := arrayValue[1 : len(arrayValue)-1]

	// split by comma and process each element
	elements := strings.Split(arrayContent, ",")
	processedElements := make([]string, 0, len(elements))

	for _, element := range elements {
		element = strings.TrimSpace(element)

		// skip empty elements
		if element == "" {
			continue
		}

		// skip numeric values
		if _, err := strconv.Atoi(element); err == nil {
			processedElements = append(processedElements, element)
			continue
		}

		// skip already quoted values
		if strings.HasPrefix(element, "'") && strings.HasSuffix(element, "'") {
			processedElements = append(processedElements, element)
			continue
		}

		// add 0x if not present and quote the value
		if !strings.HasPrefix(element, "0x") {
			element = "0x" + element
		}
		processedElements = append(processedElements, "'"+element+"'")
	}

	return variable + " " + operator + " [" + strings.Join(processedElements, ", ") + "]"
}

func handleSingleValue(variable, operator, value string) string {
	// skip numeric values
	if _, err := strconv.Atoi(value); err == nil {
		return variable + " " + operator + " " + value
	}

	// skip already quoted values
	if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		return variable + " " + operator + " " + value
	}

	// add 0x if not present
	if !strings.HasPrefix(value, "0x") {
		value = "0x" + value
	}

	return variable + " " + operator + " '" + value + "'"
}
