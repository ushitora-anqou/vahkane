package runner

import (
	"errors"
	"fmt"

	vahkanev1 "github.com/ushitora-anqou/vahkane/api/v1"
	"sigs.k8s.io/yaml"
)

func matchActions(
	actions []vahkanev1.DiscordInteractionAction,
	data interface{},
) (*vahkanev1.DiscordInteractionAction, error) {
	for _, action := range actions {
		var pattern interface{}
		if err := yaml.Unmarshal([]byte(action.Pattern), &pattern); err != nil {
			return nil, fmt.Errorf("failed to parse action pattern: %w", err)
		}
		if doesPatternMatch(pattern, data) {
			return &action, nil
		}
	}
	return nil, errors.New("not found")
}

func doesPatternMatch(pattern interface{}, data interface{}) bool {
	type element struct {
		pattern, data interface{}
	}
	queue := []element{}
	queue = append(queue, element{pattern, data})
	for len(queue) > 0 {
		head := queue[0]
		queue = queue[1:]

		switch pattern := head.pattern.(type) {
		case string:
			data, ok := head.data.(string)
			if !ok || pattern != data {
				return false
			}
		case int:
			data, ok := head.data.(int)
			if !ok || pattern != data {
				return false
			}
		case float64:
			data, ok := head.data.(float64)
			if !ok || pattern != data {
				return false
			}

		case map[string]interface{}:
			data, ok := head.data.(map[string]interface{})
			if !ok {
				return false
			}
			for key := range pattern {
				valueData, ok := data[key]
				if !ok {
					return false
				}
				queue = append(queue, element{pattern: pattern[key], data: valueData})
			}

		case []interface{}:
			data, ok := head.data.([]interface{})
			if !ok || len(pattern) != len(data) {
				return false
			}
			for i := range pattern {
				queue = append(queue, element{pattern: pattern[i], data: data[i]})
			}

		default:
			return false
		}
	}

	return true
}
