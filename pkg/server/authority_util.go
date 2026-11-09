package server

func hasScope(scopes []string, required string) bool {
	for _, scope := range scopes {
		if scope == required || scope == "mcp" {
			return true
		}
	}
	return false
}

func validAuthorityHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for index := 0; index < len(value); index++ {
		char := value[index]
		if char >= '0' && char <= '9' || char >= 'a' && char <= 'f' || char >= 'A' && char <= 'F' {
			continue
		}
		return false
	}
	return true
}
