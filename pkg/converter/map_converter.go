package converter

func MapKeysToSlice[K comparable, T any](m map[K]T) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func DeepCopy(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		if vMap, ok := v.(map[string]interface{}); ok {
			dst[k] = DeepCopy(vMap)
		} else {
			dst[k] = v
		}
	}
	return dst
}
