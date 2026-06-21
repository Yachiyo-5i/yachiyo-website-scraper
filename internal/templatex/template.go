package templatex

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var placeholderRE = regexp.MustCompile(`\{([^{}]+)\}`)
var wholePlaceholderRE = regexp.MustCompile(`^\{([^{}]+)\}$`)

func Render(input string, vars map[string]string) (string, error) {
	out, err := RenderString(input, stringVars(vars))
	if err != nil {
		return "", err
	}
	return out, nil
}

func RenderString(input string, vars map[string]interface{}) (string, error) {
	rendered, err := renderString(input, vars)
	if err != nil {
		return "", err
	}
	return rendered, nil
}

func RenderAny(value interface{}, vars map[string]interface{}) (interface{}, error) {
	switch v := value.(type) {
	case string:
		return renderValue(v, vars)
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))
		for key, child := range v {
			rendered, err := RenderAny(child, vars)
			if err != nil {
				return nil, err
			}
			out[key] = rendered
		}
		return out, nil
	case []interface{}:
		out := make([]interface{}, 0, len(v))
		for _, child := range v {
			rendered, err := RenderAny(child, vars)
			if err != nil {
				return nil, err
			}
			out = append(out, rendered)
		}
		return out, nil
	default:
		return value, nil
	}
}

func renderValue(input string, vars map[string]interface{}) (interface{}, error) {
	if match := wholePlaceholderRE.FindStringSubmatch(input); len(match) == 2 {
		value, ok := lookup(match[1], vars)
		if !ok {
			return nil, fmt.Errorf("missing template variables: %s", match[1])
		}
		return value, nil
	}
	return renderString(input, vars)
}

func renderString(input string, vars map[string]interface{}) (string, error) {
	var missing []string
	out := placeholderRE.ReplaceAllStringFunc(input, func(token string) string {
		key := strings.TrimSuffix(strings.TrimPrefix(token, "{"), "}")
		value, ok := lookup(key, vars)
		if !ok {
			missing = append(missing, key)
			return token
		}
		return stringify(value)
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("missing template variables: %s", strings.Join(missing, ", "))
	}
	return out, nil
}

func lookup(key string, vars map[string]interface{}) (interface{}, bool) {
	if strings.HasPrefix(key, "env.") {
		return os.Getenv(strings.TrimPrefix(key, "env.")), true
	}
	value, ok := vars[key]
	return value, ok
}

func stringify(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []string:
		return strings.Join(typed, ",")
	default:
		return fmt.Sprint(typed)
	}
}

func stringVars(vars map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(vars))
	for k, v := range vars {
		out[k] = v
	}
	return out
}
