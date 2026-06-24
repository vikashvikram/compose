package runner

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

var (
	colors = []string{
		"\033[36m", // Cyan
		"\033[32m", // Green
		"\033[33m", // Yellow
		"\033[34m", // Blue
		"\033[35m", // Magenta
		"\033[31m", // Red
	}
	colorReset = "\033[0m"
)

// StreamLogsAndWatch streams logs for all started containers in parallel and stops containers on Ctrl+C.
func (r *Runner) StreamLogsAndWatch(containerNames []string) error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	var wg sync.WaitGroup
	ctxCancelChan := make(chan struct{})

	// Start log streaming for each container
	for i, name := range containerNames {
		color := colors[i%len(colors)]
		// Get service name from container name (e.g. project_service_1 -> service)
		svcName := getServiceName(name, r.ProjectName)

		wg.Add(1)
		go func(cName, labelColor, label string) {
			defer wg.Done()
			r.streamContainerLogs(cName, labelColor, label, ctxCancelChan)
		}(name, color, svcName)
	}

	// Wait for interrupt signal
	select {
	case sig := <-sigChan:
		fmt.Printf("\nReceived signal %v. Gracefully stopping containers...\n", sig)
		close(ctxCancelChan)
		// Stop the containers
		_ = r.Stop()
	case <-waitAllDone(&wg):
		fmt.Println("All containers have exited.")
	}

	return nil
}

// streamContainerLogs executes 'container logs -f' and prints output line by line.
func (r *Runner) streamContainerLogs(containerName, color, label string, cancelChan chan struct{}) {
	cmd := exec.Command("container", "logs", "-f", containerName)
	
	// Create pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	cmd.Stderr = cmd.Stdout // Redirect stderr to stdout

	if err := cmd.Start(); err != nil {
		return
	}

	// Goroutine to kill process if canceled
	go func() {
		<-cancelChan
		_ = cmd.Process.Kill()
	}()

	reader := bufio.NewReader(stdout)
	prefix := fmt.Sprintf("%s%-10s |%s ", color, label+"_1", colorReset)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				// error reading
			}
			break
		}
		fmt.Print(prefix + line)
	}

	_ = cmd.Wait()
}

// Logs shows logs for specified services (or all if empty).
func (r *Runner) Logs(follow bool, serviceNames []string) error {
	containers, err := r.getProjectContainers()
	if err != nil {
		return err
	}

	// Filter containers
	var targetContainers []ContainerInfo
	for _, c := range containers {
		svc := c.Configuration.Labels["com.apple.compose.service"]
		if len(serviceNames) == 0 {
			targetContainers = append(targetContainers, c)
		} else {
			for _, name := range serviceNames {
				if svc == name {
					targetContainers = append(targetContainers, c)
					break
				}
			}
		}
	}

	if len(targetContainers) == 0 {
		fmt.Println("No matching containers found.")
		return nil
	}

	if follow {
		var names []string
		for _, c := range targetContainers {
			names = append(names, c.ID)
		}
		return r.StreamLogsAndWatch(names)
	}

	// Print historical logs
	var wg sync.WaitGroup
	for i, c := range targetContainers {
		color := colors[i%len(colors)]
		svcName := c.Configuration.Labels["com.apple.compose.service"]

		wg.Add(1)
		go func(cName, labelColor, label string) {
			defer wg.Done()
			cmd := exec.Command("container", "logs", cName)
			out, err := cmd.Output()
			if err != nil {
				return
			}
			prefix := fmt.Sprintf("%s%-10s |%s ", labelColor, label+"_1", colorReset)
			lines := strings.Split(string(out), "\n")
			for _, line := range lines {
				if line == "" {
					continue
				}
				fmt.Println(prefix + line)
			}
		}(c.ID, color, svcName)
	}
	wg.Wait()
	return nil
}

// getServiceName parses service name from container name.
func getServiceName(containerName, projectName string) string {
	// e.g. myproject_web_1 -> web
	prefix := projectName + "_"
	if strings.HasPrefix(containerName, prefix) {
		name := containerName[len(prefix):]
		if idx := strings.LastIndex(name, "_"); idx != -1 {
			return name[:idx]
		}
		return name
	}
	return containerName
}

// waitAllDone returns a channel that is closed when the waitgroup finishes.
func waitAllDone(wg *sync.WaitGroup) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		wg.Wait()
		close(ch)
	}()
	return ch
}
