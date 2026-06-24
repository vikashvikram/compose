package parser

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// ComposeConfig represents the root of a docker-compose.yml file.
type ComposeConfig struct {
	Version  string                   `yaml:"version"`
	Services map[string]ServiceConfig `yaml:"services"`
	Volumes  map[string]VolumeConfig  `yaml:"volumes"`
	Networks map[string]NetworkConfig `yaml:"networks"`
}

// ServiceConfig represents the configuration of a single service.
type ServiceConfig struct {
	Image         string        `yaml:"image"`
	Build         *BuildConfig  `yaml:"build"`
	ContainerName string        `yaml:"container_name"`
	Environment   EnvMap        `yaml:"environment"`
	EnvFile       StringOrSlice `yaml:"env_file"`
	Ports         []string      `yaml:"ports"`
	Volumes       []string      `yaml:"volumes"`
	DependsOn     DependsOnList `yaml:"depends_on"`
	Command       CommandList   `yaml:"command"`
	Entrypoint    CommandList   `yaml:"entrypoint"`
	User          string        `yaml:"user"`
	WorkingDir    string        `yaml:"working_dir"`
	Deploy        *DeployConfig `yaml:"deploy"`
	CapAdd        []string      `yaml:"cap_add"`
	CapDrop       []string      `yaml:"cap_drop"`
	Init          bool          `yaml:"init"`
	ReadOnly      bool          `yaml:"read_only"`
	Platform      string        `yaml:"platform"`
}

// BuildConfig represents the build configuration for a service.
type BuildConfig struct {
	Context    string            `yaml:"context"`
	Dockerfile string            `yaml:"dockerfile"`
	Args       map[string]string `yaml:"args"`
}

// UnmarshalYAML custom unmarshaler for BuildConfig because it can be a string (context) or a map.
func (b *BuildConfig) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err == nil {
		b.Context = s
		return nil
	}

	type Alias BuildConfig
	var a Alias
	if err := value.Decode(&a); err != nil {
		return err
	}
	*b = BuildConfig(a)
	return nil
}

// VolumeConfig represents the volume configuration.
type VolumeConfig struct {
	Name     string            `yaml:"name"`
	External bool              `yaml:"external"`
	Labels   map[string]string `yaml:"labels"`
}

// UnmarshalYAML custom unmarshaler for VolumeConfig since it can be empty/null in yaml.
func (v *VolumeConfig) UnmarshalYAML(value *yaml.Node) error {
	type Alias VolumeConfig
	var a Alias
	if err := value.Decode(&a); err != nil {
		// If it's a null or empty node, we just succeed
		return nil
	}
	*v = VolumeConfig(a)
	return nil
}

// NetworkConfig represents the network configuration.
type NetworkConfig struct {
	Name     string            `yaml:"name"`
	External bool              `yaml:"external"`
	Internal bool              `yaml:"internal"`
	Labels   map[string]string `yaml:"labels"`
	Driver   string            `yaml:"driver"`
}

// UnmarshalYAML custom unmarshaler for NetworkConfig.
func (n *NetworkConfig) UnmarshalYAML(value *yaml.Node) error {
	type Alias NetworkConfig
	var a Alias
	if err := value.Decode(&a); err != nil {
		return nil
	}
	*n = NetworkConfig(a)
	return nil
}

// EnvMap represents environment variables which can be a map or a list of KEY=VAL.
type EnvMap map[string]string

func (e *EnvMap) UnmarshalYAML(value *yaml.Node) error {
	*e = make(EnvMap)

	// Try map first
	var m map[string]string
	if err := value.Decode(&m); err == nil {
		*e = m
		return nil
	}

	// Try slice next
	var s []string
	if err := value.Decode(&s); err == nil {
		for _, item := range s {
			parts := strings.SplitN(item, "=", 2)
			if len(parts) == 1 {
				// KEY (inherit from host)
				(*e)[parts[0]] = os.Getenv(parts[0])
			} else {
				(*e)[parts[0]] = parts[1]
			}
		}
		return nil
	}

	return fmt.Errorf("failed to unmarshal environment map/slice at line %d", value.Line)
}

// StringOrSlice represents a field that can be a single string or a slice of strings.
type StringOrSlice []string

func (ss *StringOrSlice) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err == nil {
		*ss = []string{s}
		return nil
	}

	var sl []string
	if err := value.Decode(&sl); err == nil {
		*ss = sl
		return nil
	}

	return fmt.Errorf("failed to unmarshal string or slice at line %d", value.Line)
}

// DependsOnList represents depends_on which can be a slice of strings or a map.
type DependsOnList []string

func (d *DependsOnList) UnmarshalYAML(value *yaml.Node) error {
	var sl []string
	if err := value.Decode(&sl); err == nil {
		*d = sl
		return nil
	}

	// If it is a map (e.g. service: { condition: service_started })
	var m map[string]interface{}
	if err := value.Decode(&m); err == nil {
		for k := range m {
			*d = append(*d, k)
		}
		return nil
	}

	return fmt.Errorf("failed to unmarshal depends_on at line %d", value.Line)
}

// CommandList represents command or entrypoint which can be a string or a slice of strings.
type CommandList []string

func (c *CommandList) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err == nil {
		// Split by spaces, or handle shell execution
		fields := strings.Fields(s)
		for i, field := range fields {
			if (strings.HasPrefix(field, "\"") && strings.HasSuffix(field, "\"")) ||
				(strings.HasPrefix(field, "'") && strings.HasSuffix(field, "'")) {
				if len(field) >= 2 {
					fields[i] = field[1 : len(field)-1]
				}
			}
		}
		*c = fields
		return nil
	}

	var sl []string
	if err := value.Decode(&sl); err == nil {
		*c = sl
		return nil
	}

	return fmt.Errorf("failed to unmarshal command list at line %d", value.Line)
}

// DeployConfig represents deployment resources like cpu/memory.
type DeployConfig struct {
	Resources ResourcesLimits `yaml:"resources"`
}

type ResourcesLimits struct {
	Limits LimitSpec `yaml:"limits"`
}

type LimitSpec struct {
	CPUs   string `yaml:"cpus"`
	Memory string `yaml:"memory"`
}

var braceRegex = regexp.MustCompile(`\$\{([a-zA-Z0-9_]+)(?::?-([^}]+))?\}`)
var simpleRegex = regexp.MustCompile(`\$([a-zA-Z0-9_]+)`)

// ParseFile parses a docker-compose.yml file, resolving environment variables.
func ParseFile(filePath string) (*ComposeConfig, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// 1. Load env vars from .env file if it exists in the same directory
	dir := filepath.Dir(filePath)
	dotEnv := loadDotEnv(filepath.Join(dir, ".env"))

	// 2. Interpolate environment variables in the raw YAML content
	interpolatedYaml := interpolate(string(data), dotEnv)

	var config ComposeConfig
	err = yaml.Unmarshal([]byte(interpolatedYaml), &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse yaml: %w", err)
	}

	return &config, nil
}

func interpolate(content string, dotEnv map[string]string) string {
	// Escape $$ first
	content = strings.ReplaceAll(content, "$$", "___DOUBLE_DOLLAR_ESC___")

	// 1. Replace braced syntax: ${VAR:-default} or ${VAR-default} or ${VAR}
	content = braceRegex.ReplaceAllStringFunc(content, func(match string) string {
		submatches := braceRegex.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		name := submatches[1]
		hasDefault := len(submatches) > 2 && submatches[2] != ""
		defaultVal := ""
		if hasDefault {
			defaultVal = submatches[2]
		}

		isDefaultIfEmpty := strings.Contains(match, ":-")

		val, found := os.LookupEnv(name)
		if !found {
			if valFromEnv, ok := dotEnv[name]; ok {
				val = valFromEnv
				found = true
			}
		}

		if isDefaultIfEmpty {
			if !found || val == "" {
				return defaultVal
			}
			return val
		} else {
			if !found {
				return defaultVal
			}
			return val
		}
	})

	// 2. Replace simple syntax: $VAR
	content = simpleRegex.ReplaceAllStringFunc(content, func(match string) string {
		submatches := simpleRegex.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		name := submatches[1]
		val, found := os.LookupEnv(name)
		if !found {
			if valFromEnv, ok := dotEnv[name]; ok {
				val = valFromEnv
				found = true
			}
		}
		if found {
			return val
		}
		return ""
	})

	content = strings.ReplaceAll(content, "___DOUBLE_DOLLAR_ESC___", "$")
	return content
}

func loadDotEnv(filePath string) map[string]string {
	env := make(map[string]string)
	file, err := os.Open(filePath)
	if err != nil {
		return env
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			// Strip optional quotes around value
			val := parts[1]
			if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) ||
				(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
				if len(val) >= 2 {
					val = val[1 : len(val)-1]
				}
			}
			env[parts[0]] = val
		}
	}
	return env
}
