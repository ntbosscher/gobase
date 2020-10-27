package strs

func Coalesce(value string, defaultValue string) string {
	if value == "" {
		return defaultValue
	}

	return value
}
