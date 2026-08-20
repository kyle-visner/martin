package martin

import (
	"fmt"
	"os"
	"strings"
)

// SecretFromEnv reads NAME, or the contents of NAME_FILE when NAME is empty.
// Docker / Compose secret mounts use the _FILE convention. A configured
// NAME_FILE that cannot be read is an error; it is never treated as an empty
// secret. The value is never logged.
func SecretFromEnv(name string) (string, error) {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value, nil
	}
	path := strings.TrimSpace(os.Getenv(name + "_FILE"))
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s_FILE: %w", name, err)
	}
	return strings.TrimSpace(string(data)), nil
}
