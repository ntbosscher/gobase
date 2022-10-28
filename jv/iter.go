package jv

type Iter[T any] struct {
	src      []T
	position int
	filter   func(input T) bool
}

func NewIter[T any](input []T) *Iter[T] {
	return &Iter[T]{
		src: input,
	}
}

func (i *Iter[T]) Next() (T, bool) {
	for {
		index := i.position
		i.position++

		if index >= len(i.src) {
			var blank T
			return blank, false
		}

		item := i.src[index]
		if i.filter != nil {
			if !i.filter(item) {
				continue
			}
		}

		return item, true
	}
}

// Filter allows items to be removed from the Iter stream
// calling .SetFilter(nil) will remove any filters
func (i *Iter[T]) SetFilter(fx func(input T) bool) {
	i.filter = fx
}
