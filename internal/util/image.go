package util

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
)

// EncodeImageToBase64 converts image bytes to a base64 string with proper content type header
func EncodeImageToBase64(imageBytes []byte) (string, error) {
	contentType := http.DetectContentType(imageBytes)
	if !strings.HasPrefix(contentType, "image/") {
		return "", fmt.Errorf("invalid image content type: %s", contentType)
	}
	base64Str := base64.StdEncoding.EncodeToString(imageBytes)
	return fmt.Sprintf("data:%s;base64,%s", contentType, base64Str), nil
}
