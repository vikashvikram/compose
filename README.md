# macOS Container Compose (`compose`)

A lightweight, native, and high-performance Docker Compose clone designed specifically for macOS. It acts as an orchestrator for multi-container applications, wrapping macOS's native `container` command-line utility.

---

## What is it?
`compose` parses a standard `docker-compose.yml` file and coordinates building, running, networking, and log multiplexing for multi-container development environments on macOS. 

Under the hood, it communicates with Apple's open-source `container` CLI utility, which leverages the native macOS `Virtualization` and `Hypervisor` frameworks.

---

## Why is this better than Docker Desktop on macOS?

1. **One-VM-Per-Container Isolation**: Docker Desktop runs all containers inside a single, shared Linux Virtual Machine. `compose` utilizes macOS's native virtualization framework to create lightweight, isolated sandboxed environments per container.
2. **Apple Silicon Optimization**: Written in Go and interfacing directly with Swift-native virtualization tools, it is optimized for ARM64 (M1/M2/M3/M4 chips) with near-zero overhead.
3. **VirtioFS File Sharing**: Instead of heavy file synchronization daemons, it mounts directories natively using VirtioFS for lightning-fast disk I/O, solving the notorious Docker-on-macOS volume mount slowdowns.
4. **Zero Background Daemons**: It doesn't require a heavy helper daemon constantly consuming RAM and CPU in the background when not in use.

---

## Prerequisites
* **macOS**: Requires macOS 15 (Sequoia) or later running on Apple Silicon.

---

## Installation

You can install `compose` along with its required dependencies using Homebrew:

```bash
# 1. Tap the repository
brew tap vikashvikram/tap

# 2. Install compose (this will automatically install the 'container' dependency)
brew install compose

# 3. Start the native macOS container background service
brew services start container
```

---

## Supported Commands

* **`compose up [-d] [--build]`**: Create and start containers in topological dependency order (respecting `depends_on`).
  * If `--build` is specified, it will build/rebuild the service images before starting the containers.
  * If `-d` (detached) is omitted, it will stream multiplexed, colorized logs from all containers in real time. Pressing `Ctrl+C` will gracefully stop the containers.
* **`compose down [-v]`**: Stop and remove containers, networks, and volumes.
* **`compose build`**: Build or rebuild service images defined with a `build` context using BuildKit.
* **`compose ps`**: List all containers for the project, showing name, image reference, state, internal IP address, and host-port mappings.
* **`compose logs [-f] [service]`**: View output logs from containers (interleaved or followed).
* **`compose exec [-T] SERVICE COMMAND`**: Execute a command inside a running container. Supports interactive TTY and can be forced to non-interactive mode using `-T` for CI/CD environments.
* **`compose stop`**: Stop running containers.
* **`compose start`**: Start stopped containers.
* **`compose restart`**: Restart containers.

---

## Naming & Labeling Conventions
To avoid conflicts with other projects, resource names and labels are systematically prefixed:
* **Container Name**: `<project_name>_<service_name>_1`
* **Network Name**: `<project_name>_default`
* **Volume Name**: `<project_name>_<volume_name>`
* **Metadata Labels**: All resources are automatically labeled with:
  * `com.apple.compose.project=<project_name>`
  * `com.apple.compose.service=<service_name>`

---

## Interpolation & Configuration Parsing
* **Environment Variable Interpolation**: Supports bash-like variable substitution in your YAML configurations (e.g. `${VAR:-default}` or `$VAR`). Variables are read from the host shell environment and local `.env` files.
* **Smart Quoting**: Automatically strips outer double/single quotes from space-separated command lists to ensure arguments pass correctly to the container runtimes.

---

## Open Source Licensing Recommendation

To meet your requirements:
1. **Free for anyone** (including enterprises for development and production work).
2. **Allows modification** and redistribution.
3. **Prevents others from selling it or commercializing it for a profit**.

We recommend the **Apache License 2.0 with the Commons Clause**.

### How it works:
* You license the code under **Apache 2.0**, which is a widely accepted, enterprise-friendly open-source license.
* You append the **Commons Clause** amendment to the license. The Commons Clause states:
  > *“...the License does not grant to you the right to Sell the Software. 'Sell' means practicing any of the rights granted to you by the License to a third party for a fee, including selling, licensing, sublicensing, or charging for access to the Software.”*
* **Why this fits perfectly**:
  * **Enterprises** can use it internally for their business operations, development, and production at no cost.
  * **Competitors** cannot package the binary, rename it, or modify it slightly to sell it as a commercial product.
