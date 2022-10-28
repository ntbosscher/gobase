package jv

type Pair[T any, U any] struct {
	Key   T
	Value U
}

type Map[T any, U any] struct {
	Pairs    []*Pair[T, U]
	equality func(a, b T) bool
}

func NewMap[T any, U any](equality func(a, b T) bool) *Map[T, U] {
	return &Map[T, U]{
		equality: equality,
	}
}

func (s *Map[T, U]) Set(key T, value U) bool {
	v := First(s.Pairs, func(input *Pair[T, U]) bool {
		return s.equality(input.Key, key)
	})

	if v == nil {
		s.Pairs = append(s.Pairs, &Pair[T, U]{
			Key:   key,
			Value: value,
		})

		return true
	}

	v.Value = value
	return false
}

func (s *Set[T]) Len() int {
	return len(s.Values)
}

func (s *Map[T, U]) Get(key T) (U, bool) {
	v := First(s.Pairs, func(input *Pair[T, U]) bool {
		return s.equality(input.Key, key)
	})

	if v == nil {
		var value U
		return value, false
	}

	return v.Value, true
}

func (s *Map[T, U]) Delete(key T) {
	s.Pairs = Filter(s.Pairs, func(input *Pair[T, U]) bool {
		return !s.equality(input.Key, key)
	})
}

func (s *Map[T, U]) Transform(key T, transform func(value U) U) {
	v, _ := s.Get(key)
	s.Set(key, transform(v))
}

func (s *Map[T, U]) Len() int {
	return len(s.Pairs)
}
