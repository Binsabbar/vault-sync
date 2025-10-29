package converter

import "fmt"

func ConvertInterfaceToInt64(value interface{}) (int64, error) {
	switch v := value.(type) {
	case float32:
		return int64(v), nil
	case float64:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	default:
		return 0, fmt.Errorf("unsupported type %T for conversion to int64", value)
	}
}
