package model

import (
	"fmt"

	"github.com/elimity-com/scim/filter"
	scim_filter_parser "github.com/scim2/filter-parser/v2"
)

func PrincipalFromFilter(filterValidator *filter.Validator) (string, error) {
	if filterValidator == nil {
		return "", nil
	}
	f, ok := filterValidator.GetFilter().(*scim_filter_parser.AttributeExpression)
	if !ok {
		return "", fmt.Errorf("only single expressions are supported")
	}
	if f.Operator != "eq" {
		return "", fmt.Errorf("only operator 'eq' is supported in filters")
	}
	if f.AttributePath.AttributeName != "userName" {
		return "", fmt.Errorf("only 'userName' is supported in filters")
	}
	return f.CompareValue.(string), nil
}
