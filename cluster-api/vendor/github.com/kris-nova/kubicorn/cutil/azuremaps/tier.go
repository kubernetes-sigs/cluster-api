package azuremaps

import "strings"

func GetTierFromSize(size string) string {
	if strings.Contains(size, "Standard") {
		return "Standard"
	}
	return "Standard"
}