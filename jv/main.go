package jv

import (
	"golang.org/x/exp/constraints"
	"strings"
)

func StringStartsWithAny(href string, s ...string) bool {
	href = strings.TrimSpace(strings.ToLower(href))

	for _, item := range s {
		item = strings.ToLower(item)
		if strings.HasPrefix(href, item) {
			return true
		}
	}

	return false
}

func StringContainsCI(input string, substring ...string) bool {
	for _, str := range substring {
		if strings.Contains(strings.ToLower(input), strings.ToLower(str)) {
			return true
		}
	}

	return false
}

func Min[T constraints.Ordered](input ...T) T {
	m := input[0]

	for _, item := range input {
		if m < item {
			m = item
		}
	}

	return m
}
