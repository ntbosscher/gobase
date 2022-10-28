package jv

import (
	"github.com/ntbosscher/gobase/randomish"
	"github.com/pkg/errors"
	"sort"
)

func ArrayItemCompare[T comparable](a []T, b []T) bool {
	if len(a) != len(b) {
		return false
	}

	for i, item := range a {
		if b[i] != item {
			return false
		}
	}

	return true
}

// SortFx sors the list given in ascending order based on the scoring function provided
func SortFx[T any](list []T, score func(input T) float64) []T {
	sort.Sort(byScore{
		length: len(list),
		score: func(i int) float64 {
			return score(list[i])
		},
		swap: func(i, j int) {
			list[i], list[j] = list[j], list[i]
		},
	})

	return list
}

type byScore struct {
	length int
	score  func(i int) float64
	swap   func(i, j int)
}

func (b byScore) Len() int {
	return b.length
}

func (b byScore) Less(i, j int) bool {
	return b.score(i) < b.score(j)
}

func (b byScore) Swap(i, j int) {
	b.swap(i, j)
}

func Reverse[T any](input []T) []T {
	var out []T
	for i := len(input) - 1; i >= 0; i-- {
		out = append(out, input[i])
	}

	return out
}

func IndexOf[T any](input []T, filter func(input T) bool) int {

	for i, item := range input {
		if filter(item) {
			return i
		}
	}

	return -1
}

func IndexOfItem[T comparable](input []T, item T) int {
	return IndexOf(input, func(it T) bool {
		return it == item
	})
}

func RandomArrayElement[T any](input []T) T {
	if len(input) == 0 {
		panic(errors.New("can't find random element of 0-length array"))
	}

	i := randomish.Int(0, len(input)-1)
	return input[i]
}

func Unique[T comparable](input []T) []T {
	mp := map[T]bool{}
	list := []T{}

	for _, item := range input {
		if mp[item] {
			continue
		}

		mp[item] = true
		list = append(list, item)
	}

	return list
}

func First[T any](input []T, filter func(input T) bool) T {

	for _, item := range input {
		if filter(item) {
			return item
		}
	}

	var val T
	return val
}

func Last[T any](input []T, filter func(input T) bool) T {

	for i := len(input) - 1; i >= 0; i-- {
		if filter(input[i]) {
			return input[i]
		}
	}

	var val T
	return val
}

func Contains[T any](input []T, filter func(input T) bool) bool {

	for _, item := range input {
		if filter(item) {
			return true
		}
	}

	return false
}

func Mapper[T any, U any](input []T, mapper func(input T) U) []U {
	var out []U

	for _, item := range input {
		out = append(out, mapper(item))
	}

	return out
}

func Reduce[T any, U any](input []T, reducer func(input T, acc U) U, init U) U {
	current := init
	for _, item := range input {
		current = reducer(item, current)
	}

	return current
}

func Union[T comparable](a []T, b []T) []T {
	var common []T

	for _, item := range a {
		if ContainsItem(b, item) {
			common = append(common, item)
		}
	}

	return common
}

func ContainsItem[T comparable](input []T, item T) bool {
	for _, v := range input {
		if v == item {
			return true
		}
	}

	return false
}

func Filter[T any](input []T, filter func(input T) bool) []T {
	var list []T

	for _, item := range input {
		if filter(item) {
			list = append(list, item)
		}
	}

	return list
}

func RemoveFromList[T comparable](src []T, toRemove []T) []T {
	var list []T
	remove := map[T]bool{}

	for _, item := range toRemove {
		remove[item] = true
	}

	for _, item := range src {
		if !remove[item] {
			list = append(list, item)
		}
	}

	return list
}

func SelectMany[T any](input [][]T) []T {
	var list []T
	for _, item := range input {
		list = append(list, item...)
	}

	return list
}

func All[T any](input []T, check func(input T) bool) bool {
	for _, item := range input {
		if !check(item) {
			return false
		}
	}

	return true
}

func Any[T any](input []T, check func(input T) bool) bool {
	for _, item := range input {
		if check(item) {
			return true
		}
	}

	return false
}

func FilterByMapKey[T comparable](list []T, toRemove map[T]bool) []T {
	var newList []T

	for _, item := range list {
		if toRemove[item] {
			continue
		}

		newList = append(newList, item)
	}

	return newList
}

func Intersection[T comparable](listA []T, listB []T) []T {
	var newList []T

	for _, a := range listA {
		for _, b := range listB {
			if a == b {
				newList = append(newList, a)
				break
			}
		}
	}

	return Unique(newList)
}

func MergeObjects[T comparable](list []T, merger func(a T, b T) (result T, merged bool)) []T {

	invalid := map[T]bool{}

	for index, a := range list {
		for _, b := range list[index+1:] {
			if invalid[b] {
				continue
			}

			rs, merged := merger(a, b)
			if merged {
				list[index] = rs
				a = rs
				invalid[b] = true
			}
		}
	}

	return RemoveFromList(list, GetMapKeys(invalid))
}