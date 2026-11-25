package telemetry

// ValidUUID reports whether value is a UUID in canonical 8-4-4-4-12 text form.
func ValidUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		switch index {
		case 8, 13, 18, 23:
			if character != '-' {
				return false
			}
		default:
			if !('0' <= character && character <= '9' || 'a' <= character && character <= 'f' || 'A' <= character && character <= 'F') {
				return false
			}
		}
	}
	return true
}
