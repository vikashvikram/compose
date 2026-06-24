package main

import (
	"fmt"
	"os"
	"strings"

	"compose/runner"
)

func usage() {
	fmt.Print(`macOS Container Compose Tool

Usage:
  compose [OPTIONS] COMMAND [ARGS...]

Options:
  -f, --file PATH          Path to compose file (default: docker-compose.yml/yaml or compose.yml/yaml)
  -p, --project-name NAME  Project name (default: containing directory name)
  -h, --help               Show this help message

Commands:
  up                       Create and start containers
                           Flags: -d, --detach   Run containers in background
                                  --build        Build images before starting containers
  down                     Stop and remove containers, networks, and volumes
                           Flags: -v, --volumes  Remove named volumes
  build                    Build or rebuild services
  exec                     Execute a command in a running container
                           Usage: compose exec SERVICE COMMAND [ARGS...]
  ps                       List containers
  logs                     View output from containers
                           Flags: -f, --follow   Follow log output
  start                    Start stopped containers
  stop                     Stop running containers
  restart                  Restart containers
`)
}

func main() {
	var fileFlag string
	var projectFlag string

	args := os.Args[1:]
	subCommand := ""
	var subArgs []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-f" || arg == "--file" {
			if i+1 < len(args) {
				fileFlag = args[i+1]
				i++
			} else {
				fmt.Println("Error: --file requires a value")
				os.Exit(1)
			}
		} else if arg == "-p" || arg == "--project-name" {
			if i+1 < len(args) {
				projectFlag = args[i+1]
				i++
			} else {
				fmt.Println("Error: --project-name requires a value")
				os.Exit(1)
			}
		} else if arg == "-h" || arg == "--help" {
			usage()
			os.Exit(0)
		} else if strings.HasPrefix(arg, "-") {
			fmt.Printf("Error: unknown option: %s\n", arg)
			usage()
			os.Exit(1)
		} else {
			subCommand = arg
			subArgs = args[i+1:]
			break
		}
	}

	if subCommand == "" {
		usage()
		os.Exit(1)
	}

	// Find compose file if not specified
	composeFile := fileFlag
	if composeFile == "" {
		var err error
		composeFile, err = findComposeFile()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	}

	// Initialize runner
	run, err := runner.NewRunner(composeFile, projectFlag)
	if err != nil {
		fmt.Printf("Error initializing compose: %v\n", err)
		os.Exit(1)
	}

	// Route subcommand
	switch subCommand {
	case "up":
		// Parse upArgs. We manually check for flags
		isDetached := false
		shouldBuild := false
		for _, arg := range subArgs {
			if arg == "-d" || arg == "--detach" {
				isDetached = true
			} else if arg == "--build" {
				shouldBuild = true
			}
		}

		if shouldBuild {
			err = run.Build()
			if err != nil {
				fmt.Printf("Error building images: %v\n", err)
				os.Exit(1)
			}
		}

		err = run.Up(isDetached)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

	case "down":
		removeVolumes := false
		for _, arg := range subArgs {
			if arg == "-v" || arg == "--volumes" {
				removeVolumes = true
			}
		}
		err = run.Down(removeVolumes)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

	case "build":
		err = run.Build()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

	case "exec":
		disableTty := false
		var cleanArgs []string
		for _, arg := range subArgs {
			if arg == "-T" {
				disableTty = true
			} else {
				cleanArgs = append(cleanArgs, arg)
			}
		}

		if len(cleanArgs) < 2 {
			fmt.Println("Error: exec requires a service name and a command.")
			fmt.Println("Usage: compose exec [-T] SERVICE COMMAND [ARGS...]")
			os.Exit(1)
		}
		serviceName := cleanArgs[0]
		command := cleanArgs[1:]
		err = run.Exec(serviceName, command, disableTty)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

	case "ps":
		err = run.Ps()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

	case "logs":
		follow := false
		var services []string
		for _, arg := range subArgs {
			if arg == "-f" || arg == "--follow" {
				follow = true
			} else if !strings.HasPrefix(arg, "-") {
				services = append(services, arg)
			}
		}
		err = run.Logs(follow, services)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

	case "stop":
		err = run.Stop()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

	case "start":
		err = run.Start()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

	case "restart":
		err = run.Restart()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Printf("Error: unknown command %s\n", subCommand)
		usage()
		os.Exit(1)
	}
}

func findComposeFile() (string, error) {
	candidates := []string{
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("no compose file found in current directory")
}
