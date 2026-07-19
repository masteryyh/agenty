package typeutil

import "strings"

func ParseBoolQueryParam(param string) bool {
	p := strings.ToLower(param)
	return p == "true" || p == "1" || p == "yes"
}
