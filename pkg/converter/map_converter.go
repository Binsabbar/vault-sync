package converter

func MapKeysToSlice[K comparable, T any](m map[K]T) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
