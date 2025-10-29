package testutil

func CopyStruct[T any](original *T) *T {
	if original == nil {
		return nil
	}
	copy := *original
	return &copy
}
