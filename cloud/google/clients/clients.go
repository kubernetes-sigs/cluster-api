package clients

import (
	"fmt"
	"strings"
)

const (
	projectNamePrefix = "projects/"
)

// Formats the given project id as a GCP consumer id, there is no validation done on the input
func GetConsumerIdForProject(projectId string) string {
	return fmt.Sprintf("project:%v", projectId)
}

func NormalizeProjectNameOrId(name string) string {
	if strings.HasPrefix(name, projectNamePrefix) {
		return name
	}
	return fmt.Sprintf("%v%v", projectNamePrefix, name)
}
