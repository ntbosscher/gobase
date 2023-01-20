package jv

func GetMapKeys[T comparable, V any](input map[T]V) []T {
	var list []T

	for k := range input {
		list = append(list, k)
	}

	return list
}
func GetMapValues[T comparable, V any](input map[T]V) []V {
	var list []V

	for _, v := range input {
		list = append(list, v)
	}

	return list
}

func MakeBoolMap[T comparable](input []T) map[T]bool {
	mp := map[T]bool{}

	for _, item := range input {
		mp[item] = true
	}

	return mp
}

func BoolMapToArray[T comparable](input map[T]bool) []T {
	var out []T
	
	for k, v := range input {
		if v {
			out = append(out, k)
		}
	}

	return out
}

func CloneMap[T comparable, V any](input map[T]V) map[T]V {
	out := map[T]V{}

	for k, v := range input {
		out[k] = v
	}

	return out
}

func GetMapPairs[T comparable, U any](input map[T]U) []*Pair[T, U] {
	var list []*Pair[T, U]

	for k, v := range input {
		list = append(list, &Pair[T, U]{
			Key:   k,
			Value: v,
		})
	}

	return list
}

func CountMap[T comparable](input []T) map[T]int {
	mp := map[T]int{}

	for _, item := range input {
		mp[item] = mp[item] + 1
	}

	return mp
}

func CloneWithKeys[T comparable, V any](input map[T]V, keys ...T) map[T]V {
	out := map[T]V{}

	for _, v := range keys {
		value, ok := input[v]
		if !ok {
			continue
		}

		out[v] = value
	}

	return out
}

func MapDiff[T comparable, V comparable](m0 map[T]V, m1 map[T]V) map[T][]V {
	out := map[T][]V{}

	for k, v := range m0 {
		if m1[k] != v {
			out[k] = []V{v, m1[k]}
		}
	}

	for k, v := range m1 {
		if m0[k] != v {
			out[k] = []V{m0[k], v}
		}
	}

	return out
}

func FilterMapByKey[T comparable, V any](input map[T]V, keys ...T) map[T]V {
	return CloneWithKeys(input, keys...)
}

func FilterMap[T comparable, V any](input map[T]V, include func(key T, value V) bool) map[T]V {
	out := map[T]V{}

	for k, v := range input {
		if include(k, v) {
			out[k] = v
		}
	}

	return out
}

func MergeMaps[T comparable, V any](maps ...map[T]V) map[T]V {
	out := map[T]V{}

	for _, mp := range maps {
		for k, v := range mp {
			out[k] = v
		}
	}

	return out
}
