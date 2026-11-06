package passkey

// mapText looks a text key up, preferring the last occurrence.
func mapText(value cborMap, key string) (any, bool) {
	for index := len(value.keys) - 1; index >= 0; index-- {
		if text, ok := value.keys[index].(string); ok && text == key {
			return value.values[index], true
		}
	}
	return nil, false
}

// mapInt looks an integer key up, preferring the last occurrence.
func mapInt(value cborMap, key int64) (any, bool) {
	for index := len(value.keys) - 1; index >= 0; index-- {
		if number, ok := value.keys[index].(int64); ok && number == key {
			return value.values[index], true
		}
	}
	return nil, false
}
