package plugin

import (
	"errors"
	"fmt"
	"strings"
)

// maxExpandedTargets limits how many element/attribute (or PI point) combinations a single query
// can expand into when multi-value template variables are used.
const maxExpandedTargets = 1000

// errTooManyTargets is returned when a query expands into more than maxExpandedTargets targets.
var errTooManyTargets = errors.New("too many targets")

// expandedValue is one combination produced by expandVariables.
type expandedValue struct {
	// Value is the text with every {a,b,...} group replaced by one of its values.
	Value string
	// Variables holds the value chosen from each group, in order of appearance.
	Variables []string
}

// variableValueDecoder decodes the characters the frontend escapes inside a multi-value group
// so that values containing commas or braces do not break the group.
var variableValueDecoder = strings.NewReplacer("%2C", ",", "%7B", "{", "%7D", "}", "%25", "%")

// expandVariables expands every {a,b,...} group in text into all combinations (cartesian product).
// Grafana formats multi-value template variables as {value1,value2,...}, so a path such as
// `\\AF\DB\{SiteA,SiteB}\{Unit1,Unit2}` expands into four paths. Text without groups, or with
// unbalanced braces, is returned unchanged as a single value.
func expandVariables(text string) []expandedValue {
	results := []expandedValue{{Value: "", Variables: []string{}}}
	rest := text
	for {
		start := strings.Index(rest, "{")
		if start == -1 {
			break
		}
		end := strings.Index(rest[start:], "}")
		if end == -1 {
			break
		}
		end += start

		prefix := rest[:start]
		options := strings.Split(rest[start+1:end], ",")
		next := make([]expandedValue, 0, len(results)*len(options))
		for _, result := range results {
			for _, option := range options {
				option = variableValueDecoder.Replace(strings.TrimSpace(option))
				variables := make([]string, len(result.Variables), len(result.Variables)+1)
				copy(variables, result.Variables)
				next = append(next, expandedValue{
					Value:     result.Value + prefix + option,
					Variables: append(variables, option),
				})
			}
		}
		results = next
		rest = rest[end+1:]
	}

	for i := range results {
		results[i].Value += rest
	}
	return results
}

// expandedTarget is one element (or PI server) path combined with one attribute (or PI point).
type expandedTarget struct {
	BasePath  string
	Attribute string
	// Variable is the label prefix for the legacy data format: the values chosen for the
	// multi-value variables of the element path, joined with a backslash.
	Variable string
	// MultiVariable is true when the element path used more than one multi-value variable.
	MultiVariable bool
}

// getExpandedTargets returns every element/attribute combination of the query after expanding
// multi-value template variables in the element path and in each attribute.
func (q *PIWebAPIQuery) getExpandedTargets() ([]expandedTarget, error) {
	if q.Target == nil {
		return []expandedTarget{}, nil
	}
	basePaths := expandVariables(q.getBasePath())

	attributes := make([]string, 0, len(q.Attributes))
	for _, attribute := range q.Attributes {
		for _, expanded := range expandVariables(attribute.Value.Value) {
			attributes = append(attributes, expanded.Value)
		}
	}

	total := len(basePaths) * len(attributes)
	if total > maxExpandedTargets {
		return nil, fmt.Errorf("%w: the template variables expand this query into %d targets (%d element paths x %d attributes), the limit is %d",
			errTooManyTargets, total, len(basePaths), len(attributes), maxExpandedTargets)
	}

	targets := make([]expandedTarget, 0, total)
	for _, basePath := range basePaths {
		for _, attribute := range attributes {
			targets = append(targets, expandedTarget{
				BasePath:      basePath.Value,
				Attribute:     attribute,
				Variable:      strings.Join(basePath.Variables, `\`),
				MultiVariable: len(basePath.Variables) > 1,
			})
		}
	}
	return targets, nil
}
