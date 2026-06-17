package controller

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
)

func GetAPICreds(secret v1.Secret, pathAPIKey, pathAPISecret string) (string, string, error) {
	apiKey, ok := secret.Data[pathAPIKey]
	if !ok {
		return "", "", fmt.Errorf("unable to find api key in data: %q", pathAPIKey)
	}

	apiSecret, ok := secret.Data[pathAPISecret]
	if !ok {
		return "", "", fmt.Errorf("unable to find api secret in data: %q", pathAPISecret)
	}

	return string(apiKey), string(apiSecret), nil
}
