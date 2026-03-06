package main

func CastSingleValue[T any](input interface{}) T {
	output, _ := input.(T)
	return output
}

func CastMultiValue[T any](input interface{}) []T {
	var out []T
	for _, i := range input.([]interface{}) {
		out = append(out, CastSingleValue[T](i))
	}
	return out
}
