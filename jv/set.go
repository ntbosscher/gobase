package jv

type Set[T any] struct {
	Values   []T
	equality func(a, b T) bool
}

func NewSet[T any](equality func(a, b T) bool) *Set[T] {
	return &Set[T]{
		equality: equality,
	}
}

func (s *Set[T]) Contains(value T) bool {
	for _, item := range s.Values {
		if s.equality(item, value) {
			return true
		}
	}

	return false
}

func (s *Set[T]) Add(value ...T) {

	value = Filter(value, func(input T) bool {
		return !s.Contains(input)
	})

	s.Values = append(s.Values, value...)
}

func (s *Set[T]) Remove(value T) {
	s.Values = Filter(s.Values, func(v T) bool {
		return !s.equality(value, v)
	})
}
