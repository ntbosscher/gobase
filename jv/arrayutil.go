package jv

import (
	"github.com/ntbosscher/gobase/randomish"
	"github.com/pkg/errors"
	"golang.org/x/exp/constraints"
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

func DetectSorting[T any, U constraints.Ordered](input []T, lookup func(input T) U) (asc bool, desc bool) {
	fx := &sortableFx2[T]{
		values: input,
		less: func(a T, b T) bool {
			return lookup(a) < lookup(b)
		},
	}

	asc = sort.IsSorted(fx)
	desc = sort.IsSorted(sort.Reverse(fx))
	return
}

type sortableFx2[T any] struct {
	values []T
	less   func(a T, b T) bool
}

func (s sortableFx2[T]) Len() int {
	return len(s.values)
}

func (s sortableFx2[T]) Less(i, j int) bool {
	return s.less(s.values[i], s.values[j])
}

func (s sortableFx2[T]) Swap(i, j int) {
	s.values[i], s.values[j] = s.values[j], s.values[i]
}

func SortFx2[T any](input []T, less func(a T, b T) bool) {
	srt := &sortableFx2[T]{
		values: input,
		less:   less,
	}

	sort.Sort(srt)
}

type sortable[T constraints.Ordered] struct {
	values []T
}

func (s sortable[T]) Len() int {
	return len(s.values)
}

func (s sortable[T]) Less(i, j int) bool {
	return s.values[i] < s.values[j]
}

func (s sortable[T]) Swap(i, j int) {
	s.values[i], s.values[j] = s.values[j], s.values[i]
}

func SortAsc[T constraints.Ordered](input []T) {
	sort.Sort(sortable[T]{values: input})
}

func SortDesc[T constraints.Ordered](input []T) {
	sort.Sort(sort.Reverse(sortable[T]{values: input}))
}

func IsSortedAsc[T constraints.Ordered](input []T) bool {
	return sort.IsSorted(sortable[T]{values: input})
}

func IsSortedDesc[T constraints.Ordered](input []T) bool {
	return sort.IsSorted(sort.Reverse(sortable[T]{values: input}))
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

func IndexesOf[T any](input []T, filter func(input T) bool) []int {

	var list []int

	for i, item := range input {
		if filter(item) {
			list = append(list, i)
		}
	}

	return list
}

func IndexesOfItem[T comparable](input []T, item T) []int {
	return IndexesOf(input, func(it T) bool {
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

func Intersection[T comparable](a []T, b []T) []T {
	var common []T

	for _, item := range a {
		if ContainsItem(b, item) {
			common = append(common, item)
		}
	}

	return common
}

func IntersectionFx[T any](a []T, b []T, compare func(a T, b T) bool) []T {
	var out []T

	for _, itemA := range a {
		found := false

		for _, itemB := range b {
			if compare(itemA, itemB) {
				found = true
				break
			}
		}

		if found {
			out = append(out, itemA)
		}
	}

	return out
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

func UnionAll[T comparable](listA []T, listB []T) []T {
	newList := listA

	for _, b := range listB {
		newList = append(newList, b)
	}

	return newList
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

func GroupBy[T any, U comparable](list []T, grouper func(input T) U) map[U][]T {
	output := map[U][]T{}

	for _, item := range list {
		key := grouper(item)
		output[key] = append(output[key], item)
	}

	return output
}

func Permutations[T any, U any](input []T, extract func(input T) []U, callback func(list []T, permutationColumn []U)) {
	if len(input) == 0 {
		return
	}

	var options [][]U

	for _, item := range input {
		options = append(options, extract(item))
	}

	var cursor *permCounter
	var root *permCounter

	// initialize with zeros, do reverse because permCounter.Value gives indexs in
	// reverse, so this will cause .Value() to give the normal index order
	for i, row := range Reverse(options) {
		if i == 0 {
			root = &permCounter{
				max: len(row) - 1,
			}

			cursor = root
			continue
		}

		cursor.next = &permCounter{
			max: len(row) - 1,
		}
		cursor = cursor.next
	}

	for {
		indexes := root.Value()
		row := selectIndexes(options, indexes)
		callback(input, row)

		if !root.Incr() {
			break
		}
	}
}

func selectIndexes[T any](input [][]T, indexes []int) []T {
	var row []T

	for i, index := range indexes {
		if index == -1 {
			var defaultValue T
			row = append(row, defaultValue)
		} else {
			row = append(row, input[i][index])
		}
	}

	return row
}

// permCounter gives this output
// [0,0]
// [0,1]
// [1,0]
// [1,1]
type permCounter struct {
	next  *permCounter
	value int
	max   int
}

func (p *permCounter) Value() []int {
	val := p.value

	if p.max < 0 {
		val = -1
	}

	if p.next != nil {
		return append(p.next.Value(), val)
	}

	return []int{val}
}

func (p *permCounter) Incr() bool {
	if p.value >= p.max {
		if p.next == nil {
			return false
		}

		p.value = 0
		return p.next.Incr()
	}

	p.value++
	return true
}

func AreAllTheSame[T comparable](input []T) bool {
	if len(input) == 0 {
		return true
	}

	main := input[0]

	for _, item := range input {
		if item != main {
			return false
		}
	}

	return true
}

func AreAllUnique[T comparable](input []T) bool {
	if len(input) == 0 {
		return true
	}

	parts := map[T]bool{}

	for _, item := range input {
		if parts[item] {
			return false
		}

		parts[item] = true
	}

	return true
}

func LastItem[T any](input []T) T {
	if len(input) == 0 {
		var value T
		return value
	}

	return input[len(input)-1]
}

func Mode[T comparable](input ...T) T {
	if len(input) == 0 {
		panic("can't calculated mode of zero-values")
	}

	counter := map[T]int{}
	max := 0
	var maxValue T

	for _, item := range input {
		val := counter[item] + 1
		counter[item] = val

		if val > max {
			max = val
			maxValue = item
		}
	}

	return maxValue
}

func MaxFx[T any, U constraints.Ordered](input []T, lookup func(input T) U) T {

	if len(input) == 0 {
		var value T
		return value
	}

	best := lookup(input[0])
	bestVal := input[0]

	for _, item := range input[1:] {
		value := lookup(item)
		if value > best {
			best = value
			bestVal = item
		}
	}

	return bestVal
}

func Average[T float64 | int | int32 | int64](input ...T) T {
	var sum T

	for _, item := range input {
		sum += item
	}

	return sum / T(len(input))
}
