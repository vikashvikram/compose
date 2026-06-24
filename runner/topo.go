package runner

import (
	"fmt"
)

// TopologicalSort returns a list of service names sorted by their startup order.
// If service A depends on service B, B will appear before A in the returned list.
// Returns an error if a circular dependency is detected.
func TopologicalSort(services map[string][]string) ([]string, error) {
	// visited states: 0 = unvisited, 1 = visiting (in current path), 2 = visited
	visited := make(map[string]int)
	var order []string

	var visit func(node string) error
	visit = func(node string) error {
		state := visited[node]
		if state == 1 {
			return fmt.Errorf("circular dependency detected involving service %s", node)
		}
		if state == 2 {
			return nil
		}

		visited[node] = 1

		deps := services[node]
		for _, dep := range deps {
			// If a dependency is not in the services map, we skip it (or assume it's external)
			if _, exists := services[dep]; !exists {
				continue
			}
			if err := visit(dep); err != nil {
				return err
			}
		}

		visited[node] = 2
		order = append(order, node)
		return nil
	}

	for node := range services {
		if visited[node] == 0 {
			if err := visit(node); err != nil {
				return nil, err
			}
		}
	}

	return order, nil
}
