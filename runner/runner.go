package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"compose/parser"
)

type Runner struct {
	Config      *parser.ComposeConfig
	ProjectName string
	ConfigDir   string
}

// NewRunner creates a new Runner instance.
func NewRunner(configFile string, projectName string) (*Runner, error) {
	absPath, err := filepath.Abs(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path of config file: %w", err)
	}

	config, err := parser.ParseFile(absPath)
	if err != nil {
		return nil, err
	}

	configDir := filepath.Dir(absPath)

	if projectName == "" {
		// Default to directory name sanitized
		dirName := filepath.Base(configDir)
		projectName = SanitizeProjectName(dirName)
	}

	return &Runner{
		Config:      config,
		ProjectName: projectName,
		ConfigDir:   configDir,
	}, nil
}

// SanitizeProjectName sanitizes the directory name to be safe for container and network names.
func SanitizeProjectName(name string) string {
	name = strings.ToLower(name)
	reg := regexp.MustCompile("[^a-z0-9_-]")
	name = reg.ReplaceAllString(name, "")
	if name == "" {
		name = "default"
	}
	return name
}

// containerCmd runs a command using the 'container' executable.
func (r *Runner) containerCmd(args []string, inheritIO bool) (string, error) {
	cmd := exec.Command("container", args...)
	if inheritIO {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		return "", err
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return stdout.String(), fmt.Errorf("command 'container %s' failed: %w, stderr: %s", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String(), nil
}

// getProjectContainers returns all containers belonging to the current project.
func (r *Runner) getProjectContainers() ([]ContainerInfo, error) {
	out, err := r.containerCmd([]string{"list", "-a", "--format", "json"}, false)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(out) == "" {
		return nil, nil
	}

	var allContainers []ContainerInfo
	if err := json.Unmarshal([]byte(out), &allContainers); err != nil {
		return nil, fmt.Errorf("failed to parse container list JSON: %w", err)
	}

	var projectContainers []ContainerInfo
	for _, c := range allContainers {
		if c.Configuration.Labels["com.apple.compose.project"] == r.ProjectName {
			projectContainers = append(projectContainers, c)
		}
	}
	return projectContainers, nil
}

type ContainerInfo struct {
	ID            string `json:"id"`
	Configuration struct {
		Labels map[string]string `json:"labels"`
		Image  struct {
			Reference string `json:"reference"`
		} `json:"image"`
		PublishedPorts []PublishedPort `json:"publishedPorts"`
	} `json:"configuration"`
	Status struct {
		State    string          `json:"state"`
		Networks []NetworkStatus `json:"networks"`
	} `json:"status"`
}

type PublishedPort struct {
	ContainerPort int    `json:"containerPort"`
	HostPort      int    `json:"hostPort"`
	HostAddress   string `json:"hostAddress"`
	Proto         string `json:"proto"`
}

type NetworkStatus struct {
	Network     string `json:"network"`
	IPv4Address string `json:"ipv4Address"`
}

// setupNetworks creates any custom networks or the default network.
func (r *Runner) setupNetworks() error {
	// 1. Determine which networks are needed
	neededNetworks := make(map[string]bool)
	hasCustomNetworks := false

	for _, svc := range r.Config.Services {
		// By default, if no network specified, we attach to default network
		neededNetworks["default"] = true
		// We'll see if there are custom networks later
		_ = svc
	}

	// Read defined networks
	for netName := range r.Config.Networks {
		neededNetworks[netName] = true
		hasCustomNetworks = true
	}

	if !hasCustomNetworks {
		neededNetworks["default"] = true
	}

	// Create networks if they don't exist
	for netName := range neededNetworks {
		fullName := netName
		if netName == "default" {
			fullName = r.ProjectName + "_default"
		} else {
			// If external network, we don't prefix it or create it
			if netConfig, ok := r.Config.Networks[netName]; ok && netConfig.External {
				if netConfig.Name != "" {
					fullName = netConfig.Name
				}
				// Skip creation of external network
				continue
			}
			fullName = r.ProjectName + "_" + netName
		}

		// Check if network exists
		_, err := r.containerCmd([]string{"network", "inspect", fullName}, false)
		if err != nil {
			// Create the network
			args := []string{"network", "create", "--label", "com.apple.compose.project=" + r.ProjectName, fullName}
			if netConfig, ok := r.Config.Networks[netName]; ok {
				if netConfig.Internal {
					args = append(args, "--internal")
				}
				if netConfig.Driver != "" {
					args = append(args, "--plugin", "container-network-"+netConfig.Driver)
				}
			}
			fmt.Printf("Creating network %s ...\n", fullName)
			_, err = r.containerCmd(args, false)
			if err != nil {
				return fmt.Errorf("failed to create network %s: %w", fullName, err)
			}
		}
	}

	return nil
}

// setupVolumes creates named volumes defined in the compose file.
func (r *Runner) setupVolumes() error {
	for volName, volConfig := range r.Config.Volumes {
		if volConfig.External {
			continue
		}

		fullName := r.ProjectName + "_" + volName
		if volConfig.Name != "" {
			fullName = volConfig.Name
		}

		// Check if volume exists
		_, err := r.containerCmd([]string{"volume", "inspect", fullName}, false)
		if err != nil {
			fmt.Printf("Creating volume %s ...\n", fullName)
			args := []string{"volume", "create", "--label", "com.apple.compose.project=" + r.ProjectName}
			args = append(args, fullName)
			_, err = r.containerCmd(args, false)
			if err != nil {
				return fmt.Errorf("failed to create volume %s: %w", fullName, err)
			}
		}
	}
	return nil
}

// Build builds images for services that have a build configuration.
func (r *Runner) Build() error {
	for name, svc := range r.Config.Services {
		if svc.Build == nil {
			continue
		}

		contextPath := svc.Build.Context
		if !filepath.IsAbs(contextPath) {
			contextPath = filepath.Join(r.ConfigDir, contextPath)
		}

		dockerfile := "Dockerfile"
		if svc.Build.Dockerfile != "" {
			dockerfile = svc.Build.Dockerfile
		}

		tag := fmt.Sprintf("%s_%s:latest", r.ProjectName, name)
		if svc.Image != "" {
			tag = svc.Image
		}

		fmt.Printf("Building service %s (image: %s)...\n", name, tag)

		args := []string{"build", "-t", tag}
		if svc.Build.Dockerfile != "" {
			args = append(args, "-f", filepath.Join(contextPath, dockerfile))
		}
		if svc.Platform != "" {
			args = append(args, "--platform", svc.Platform)
		}

		for k, v := range svc.Build.Args {
			args = append(args, "--build-arg", fmt.Sprintf("%s=%s", k, v))
		}

		args = append(args, contextPath)

		_, err := r.containerCmd(args, true)
		if err != nil {
			return fmt.Errorf("failed to build image for service %s: %w", name, err)
		}
	}
	return nil
}
