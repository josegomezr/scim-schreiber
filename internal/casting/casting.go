package casting

func SingleValue[T any](input interface{}) T {
	output, _ := input.(T)
	return output
}

func MultiValue[T any](input interface{}) []T {
	var out []T
	for _, i := range input.([]interface{}) {
		out = append(out, SingleValue[T](i))
	}
	return out
}
