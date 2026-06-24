package runner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Up creates and starts the containers in the correct dependency order.
func (r *Runner) Up(detach bool) error {
	// 1. Setup networks and volumes
	if err := r.setupNetworks(); err != nil {
		return err
	}
	if err := r.setupVolumes(); err != nil {
		return err
	}

	// 2. Determine topological order of services
	servicesDeps := make(map[string][]string)
	for name, svc := range r.Config.Services {
		servicesDeps[name] = svc.DependsOn
	}

	order, err := TopologicalSort(servicesDeps)
	if err != nil {
		return fmt.Errorf("failed to sort services by dependencies: %w", err)
	}

	startedContainers := []string{}

	// 3. Start containers in order
	for _, name := range order {
		svc := r.Config.Services[name]
		containerName := fmt.Sprintf("%s_%s_1", r.ProjectName, name)

		// Check if container already exists
		existing, err := r.getContainerStatus(containerName)
		if err == nil && existing != "" {
			fmt.Printf("Container %s already exists. Stopping and removing it...\n", containerName)
			// Stop if running
			if existing == "running" {
				_, _ = r.containerCmd([]string{"stop", containerName}, false)
			}
			_, _ = r.containerCmd([]string{"delete", containerName}, false)
		}

		// Determine image name
		image := svc.Image
		if svc.Build != nil && image == "" {
			image = fmt.Sprintf("%s_%s:latest", r.ProjectName, name)
		}

		fmt.Printf("Starting service %s (container: %s) ... \n", name, containerName)

		// Build run command arguments
		runArgs := []string{"run", "-d", "--name", containerName}

		if svc.Platform != "" {
			runArgs = append(runArgs, "--platform", svc.Platform)
			if strings.Contains(svc.Platform, "amd64") || strings.Contains(svc.Platform, "x86_64") {
				runArgs = append(runArgs, "--rosetta")
			}
		}

		// Labels
		runArgs = append(runArgs, "-l", "com.apple.compose.project="+r.ProjectName)
		runArgs = append(runArgs, "-l", "com.apple.compose.service="+name)

		// Network
		networkName := r.ProjectName + "_default"
		// In docker-compose, a container is attached to <project>_default by default
		runArgs = append(runArgs, "--network", networkName)

		// Merge Env files and Environment
		mergedEnv := make(map[string]string)

		// Parse Env Files
		for _, envFile := range svc.EnvFile {
			envFilePath := envFile
			if !filepath.IsAbs(envFilePath) {
				envFilePath = filepath.Join(r.ConfigDir, envFilePath)
			}
			envVars, err := parseEnvFile(envFilePath)
			if err == nil {
				for k, v := range envVars {
					mergedEnv[k] = v
				}
			} else {
				fmt.Printf("Warning: failed to read env file %s: %v\n", envFilePath, err)
			}
		}

		// Overlay Environment
		for k, v := range svc.Environment {
			mergedEnv[k] = v
		}

		for k, v := range mergedEnv {
			runArgs = append(runArgs, "-e", fmt.Sprintf("%s=%s", k, v))
		}

		// Ports
		for _, port := range svc.Ports {
			runArgs = append(runArgs, "-p", port)
		}

		// Volumes
		for _, vol := range svc.Volumes {
			parts := strings.SplitN(vol, ":", 3)
			if len(parts) < 2 {
				continue
			}
			hostPart := parts[0]
			containerPart := parts[1]
			optPart := ""
			if len(parts) == 3 {
				optPart = parts[2]
			}

			// Check if hostPart is a named volume or host path
			var resolvedHost string
			if _, isNamed := r.Config.Volumes[hostPart]; isNamed {
				// Named volume
				resolvedHost = r.ProjectName + "_" + hostPart
			} else {
				// Host path (relative or absolute)
				if strings.HasPrefix(hostPart, ".") || strings.HasPrefix(hostPart, "~") || strings.HasPrefix(hostPart, "/") {
					resolvedHost = hostPart
					if strings.HasPrefix(hostPart, "~") {
						homeDir, _ := os.UserHomeDir()
						resolvedHost = filepath.Join(homeDir, hostPart[1:])
					} else if !filepath.IsAbs(resolvedHost) {
						resolvedHost = filepath.Join(r.ConfigDir, resolvedHost)
					}
				} else {
					// Treat it as named volume if it doesn't look like path, fallback to project-prefixed named volume
					resolvedHost = r.ProjectName + "_" + hostPart
				}
			}

			mountSpec := fmt.Sprintf("%s:%s", resolvedHost, containerPart)
			if optPart != "" {
				mountSpec = fmt.Sprintf("%s:%s:%s", resolvedHost, containerPart, optPart)
			}
			runArgs = append(runArgs, "-v", mountSpec)
		}

		// Resource Limits
		if svc.Deploy != nil && svc.Deploy.Resources.Limits.CPUs != "" {
			runArgs = append(runArgs, "-c", svc.Deploy.Resources.Limits.CPUs)
		}
		if svc.Deploy != nil && svc.Deploy.Resources.Limits.Memory != "" {
			runArgs = append(runArgs, "-m", svc.Deploy.Resources.Limits.Memory)
		}

		// Capabilities
		for _, cap := range svc.CapAdd {
			runArgs = append(runArgs, "--cap-add", cap)
		}
		for _, cap := range svc.CapDrop {
			runArgs = append(runArgs, "--cap-drop", cap)
		}

		// Init
		if svc.Init {
			runArgs = append(runArgs, "--init")
		}

		// Read Only
		if svc.ReadOnly {
			runArgs = append(runArgs, "--read-only")
		}

		// Entrypoint
		if len(svc.Entrypoint) > 0 {
			// In macOS container tool, entrypoint takes a single string or is handled as option.
			// The schema doc: `--entrypoint <cmd>: Override the entrypoint of the image`
			runArgs = append(runArgs, "--entrypoint", svc.Entrypoint[0])
		}

		// User
		if svc.User != "" {
			runArgs = append(runArgs, "-u", svc.User)
		}

		// WorkingDir
		if svc.WorkingDir != "" {
			runArgs = append(runArgs, "-w", svc.WorkingDir)
		}

		// Image
		runArgs = append(runArgs, image)

		// Command arguments
		if len(svc.Entrypoint) > 0 && len(svc.Command) > 0 {
			// If entrypoint is overridden, we append command as args to entrypoint
			runArgs = append(runArgs, svc.Command...)
		} else if len(svc.Command) > 0 {
			runArgs = append(runArgs, svc.Command...)
		}

		_, err = r.containerCmd(runArgs, false)
		if err != nil {
			return fmt.Errorf("failed to run container for service %s: %w", name, err)
		}

		startedContainers = append(startedContainers, containerName)
		fmt.Printf("Service %s started successfully.\n", name)
	}

	if !detach {
		// If attached mode, we stream logs and watch for termination/signals
		fmt.Println("Attaching to container logs... Press Ctrl+C to stop.")
		return r.StreamLogsAndWatch(startedContainers)
	}

	return nil
}

// Down stops and removes all containers, networks, and volumes for this project.
func (r *Runner) Down(removeVolumes bool) error {
	containers, err := r.getProjectContainers()
	if err != nil {
		return err
	}

	// 1. Stop and delete containers
	for _, c := range containers {
		fmt.Printf("Stopping container %s ...\n", c.ID)
		if c.Status.State == "running" {
			_, _ = r.containerCmd([]string{"stop", c.ID}, false)
		}
		fmt.Printf("Removing container %s ...\n", c.ID)
		_, _ = r.containerCmd([]string{"delete", c.ID}, false)
	}

	// 2. Delete networks
	// Find networks with project label
	out, err := r.containerCmd([]string{"network", "list", "--format", "json"}, false)
	if err == nil && strings.TrimSpace(out) != "" {
		var nets []struct {
			ID            string `json:"id"`
			Configuration struct {
				Name   string            `json:"name"`
				Labels map[string]string `json:"labels"`
			} `json:"configuration"`
		}
		if json.Unmarshal([]byte(out), &nets) == nil {
			for _, n := range nets {
				if n.Configuration.Labels["com.apple.compose.project"] == r.ProjectName {
					fmt.Printf("Removing network %s ...\n", n.Configuration.Name)
					_, _ = r.containerCmd([]string{"network", "delete", n.Configuration.Name}, false)
				}
			}
		}
	}

	// 3. Delete volumes if requested
	if removeVolumes {
		out, err := r.containerCmd([]string{"volume", "list", "--format", "json"}, false)
		if err == nil && strings.TrimSpace(out) != "" {
			var vols []struct {
				ID            string `json:"id"`
				Configuration struct {
					Name   string            `json:"name"`
					Labels map[string]string `json:"labels"`
				} `json:"configuration"`
			}
			if json.Unmarshal([]byte(out), &vols) == nil {
				for _, v := range vols {
					if v.Configuration.Labels["com.apple.compose.project"] == r.ProjectName {
						fmt.Printf("Removing volume %s ...\n", v.Configuration.Name)
						_, _ = r.containerCmd([]string{"volume", "delete", v.Configuration.Name}, false)
					}
				}
			}
		}
	}

	return nil
}

// Stop stops the containers belonging to the project.
func (r *Runner) Stop() error {
	containers, err := r.getProjectContainers()
	if err != nil {
		return err
	}
	for _, c := range containers {
		if c.Status.State == "running" {
			fmt.Printf("Stopping container %s ...\n", c.ID)
			_, err := r.containerCmd([]string{"stop", c.ID}, false)
			if err != nil {
				fmt.Printf("Error stopping %s: %v\n", c.ID, err)
			}
		}
	}
	return nil
}

// Start starts stopped containers belonging to the project.
func (r *Runner) Start() error {
	containers, err := r.getProjectContainers()
	if err != nil {
		return err
	}
	for _, c := range containers {
		if c.Status.State != "running" {
			fmt.Printf("Starting container %s ...\n", c.ID)
			_, err := r.containerCmd([]string{"start", c.ID}, false)
			if err != nil {
				fmt.Printf("Error starting %s: %v\n", c.ID, err)
			}
		}
	}
	return nil
}

// Restart restarts containers belonging to the project.
func (r *Runner) Restart() error {
	containers, err := r.getProjectContainers()
	if err != nil {
		return err
	}
	for _, c := range containers {
		fmt.Printf("Restarting container %s ...\n", c.ID)
		if c.Status.State == "running" {
			_, _ = r.containerCmd([]string{"stop", c.ID}, false)
		}
		_, err := r.containerCmd([]string{"start", c.ID}, false)
		if err != nil {
			fmt.Printf("Error starting %s: %v\n", c.ID, err)
		}
	}
	return nil
}

// Ps lists all containers belonging to the project.
func (r *Runner) Ps() error {
	containers, err := r.getProjectContainers()
	if err != nil {
		return err
	}

	if len(containers) == 0 {
		fmt.Println("No containers found for this project.")
		return nil
	}

	fmt.Printf("%-35s %-30s %-10s %-15s %s\n", "NAME", "IMAGE", "STATE", "IP", "PORTS")
	fmt.Println(strings.Repeat("-", 110))
	for _, c := range containers {
		ip := "-"
		if len(c.Status.Networks) > 0 {
			ip = c.Status.Networks[0].IPv4Address
			if idx := strings.Index(ip, "/"); idx != -1 {
				ip = ip[:idx]
			}
		}

		portsStr := "-"
		if len(c.Configuration.PublishedPorts) > 0 {
			var portsList []string
			for _, p := range c.Configuration.PublishedPorts {
				portsList = append(portsList, fmt.Sprintf("%s:%d->%d/%s", p.HostAddress, p.HostPort, p.ContainerPort, p.Proto))
			}
			portsStr = strings.Join(portsList, ", ")
		}

		fmt.Printf("%-35s %-30s %-10s %-15s %s\n", c.ID, c.Configuration.Image.Reference, c.Status.State, ip, portsStr)
	}
	return nil
}

// Exec runs a command interactively in the container of a service.
func (r *Runner) Exec(serviceName string, command []string, disableTty bool) error {
	containerName := fmt.Sprintf("%s_%s_1", r.ProjectName, serviceName)
	args := []string{"exec"}

	isTTY := false
	if !disableTty {
		fileInfo, err := os.Stdin.Stat()
		if err == nil {
			if (fileInfo.Mode() & os.ModeCharDevice) != 0 {
				isTTY = true
			}
		}
	}

	if isTTY {
		args = append(args, "-it")
	} else {
		args = append(args, "-i")
	}

	args = append(args, containerName)
	args = append(args, command...)
	_, err := r.containerCmd(args, true)
	return err
}

// getContainerStatus returns the status of a container by name.
func (r *Runner) getContainerStatus(name string) (string, error) {
	out, err := r.containerCmd([]string{"inspect", name}, false)
	if err != nil {
		return "", err
	}

	var infos []ContainerInfo
	if err := json.Unmarshal([]byte(out), &infos); err != nil {
		return "", err
	}

	if len(infos) == 0 {
		return "", fmt.Errorf("no container info returned")
	}

	return infos[0].Status.State, nil
}

// parseEnvFile reads a key=value env file, skipping comments and empty lines.
func parseEnvFile(filePath string) (map[string]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	env := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}
	return env, scanner.Err()
}
