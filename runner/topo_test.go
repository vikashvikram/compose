package runner

import (
	"testing"
)

func TestTopologicalSort(t *testing.T) {
	tests := []struct {
		name      string
		services  map[string][]string
		want      []string
		wantError bool
	}{
		{
			name: "simple chain",
			services: map[string][]string{
				"web": {"app"},
				"app": {"db"},
				"db":  {},
			},
			want:      []string{"db", "app", "web"},
			wantError: false,
		},
		{
			name: "multiple dependencies",
			services: map[string][]string{
				"web":   {"app", "cache"},
				"app":   {"db"},
				"cache": {},
				"db":    {},
			},
			want:      []string{"db", "app", "cache", "web"}, // note: db and cache can be in either order as long as they precede apps
			wantError: false,
		},
		{
			name: "circular dependency",
			services: map[string][]string{
				"web": {"app"},
				"app": {"db"},
				"db":  {"web"},
			},
			want:      nil,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TopologicalSort(tt.services)
			if (err != nil) != tt.wantError {
				t.Fatalf("TopologicalSort() error = %v, wantError %v", err, tt.wantError)
			}
			if tt.wantError {
				return
			}

			// Validate order: for each service, its dependencies must appear before it in 'got'
			pos := make(map[string]int)
			for i, name := range got {
				pos[name] = i
			}

			for node, deps := range tt.services {
				nodePos, exists := pos[node]
				if !exists {
					t.Fatalf("Service %s missing from sorted order", node)
				}
				for _, dep := range deps {
					depPos, depExists := pos[dep]
					if depExists && depPos >= nodePos {
						t.Errorf("Dependency %s (pos %d) does not appear before %s (pos %d)", dep, depPos, node, nodePos)
					}
				}
			}
		})
	}
}
