package developer

import (
	"os"
	"path/filepath"

	appcontext "github.com/AdeshDeshmukh/cadence/internal/context"
)

// Ecosystem represents the detected project technology or language stack.
type Ecosystem string

const (
	EcosystemGo         Ecosystem = "go"
	EcosystemNode       Ecosystem = "node"
	EcosystemRust       Ecosystem = "rust"
	EcosystemPython     Ecosystem = "python"
	EcosystemMake       Ecosystem = "make"
	EcosystemDocker     Ecosystem = "docker"
	EcosystemGenericGit Ecosystem = "git"
	EcosystemUnknown    Ecosystem = "unknown"
)

// ProjectInfo describes the detected developer project in a directory.
type ProjectInfo struct {
	RootPath       string    `json:"root_path"`
	Ecosystem      Ecosystem `json:"ecosystem"`
	PackageManager string    `json:"package_manager,omitempty"` // "go", "pnpm", "yarn", "npm", "cargo", "pip", "poetry", "uv"
	TestRunner     string    `json:"test_runner,omitempty"`     // "go test", "npm test", "pytest", "cargo test", "make test"
	BuildTool      string    `json:"build_tool,omitempty"`
	GitRepo        bool      `json:"git_repo"`
	GitBranch      string    `json:"git_branch,omitempty"`
}

// DetectProject inspects the directory hierarchy upwards to locate project markers and determine stack.
func DetectProject(dir string) ProjectInfo {
	if dir == "" {
		if wd, err := os.Getwd(); err == nil {
			dir = wd
		}
	}

	curr := filepath.Clean(dir)
	var rootPath string
	eco := EcosystemUnknown
	pkgMgr := ""
	testRunner := ""
	buildTool := ""

	// Walk upwards looking for primary project manifests
	searchDir := curr
	for {
		// 1. Go
		if fileExists(filepath.Join(searchDir, "go.mod")) {
			eco = EcosystemGo
			pkgMgr = "go"
			testRunner = "go test ./..."
			buildTool = "go build ./..."
			rootPath = searchDir
			break
		}

		// 2. Rust
		if fileExists(filepath.Join(searchDir, "Cargo.toml")) {
			eco = EcosystemRust
			pkgMgr = "cargo"
			testRunner = "cargo test"
			buildTool = "cargo build"
			rootPath = searchDir
			break
		}

		// 3. Node / TypeScript / JavaScript
		if fileExists(filepath.Join(searchDir, "package.json")) {
			eco = EcosystemNode
			pkgMgr = "npm"
			if fileExists(filepath.Join(searchDir, "pnpm-lock.yaml")) {
				pkgMgr = "pnpm"
			} else if fileExists(filepath.Join(searchDir, "yarn.lock")) {
				pkgMgr = "yarn"
			} else if fileExists(filepath.Join(searchDir, "bun.lockb")) || fileExists(filepath.Join(searchDir, "bun.lock")) {
				pkgMgr = "bun"
			}
			testRunner = pkgMgr + " test"
			buildTool = pkgMgr + " run build"
			rootPath = searchDir
			break
		}

		// 4. Python
		if fileExists(filepath.Join(searchDir, "pyproject.toml")) ||
			fileExists(filepath.Join(searchDir, "requirements.txt")) ||
			fileExists(filepath.Join(searchDir, "Pipfile")) ||
			fileExists(filepath.Join(searchDir, "setup.py")) {
			eco = EcosystemPython
			pkgMgr = "pip"
			if fileExists(filepath.Join(searchDir, "poetry.lock")) {
				pkgMgr = "poetry"
			} else if fileExists(filepath.Join(searchDir, "uv.lock")) {
				pkgMgr = "uv"
			}
			testRunner = "pytest"
			buildTool = "python -m build"
			rootPath = searchDir
			break
		}

		// 5. Makefile
		if fileExists(filepath.Join(searchDir, "Makefile")) || fileExists(filepath.Join(searchDir, "makefile")) {
			eco = EcosystemMake
			pkgMgr = "make"
			testRunner = "make test"
			buildTool = "make build"
			rootPath = searchDir
			break
		}

		// 6. Dockerfile
		if fileExists(filepath.Join(searchDir, "Dockerfile")) || fileExists(filepath.Join(searchDir, "docker-compose.yml")) {
			eco = EcosystemDocker
			pkgMgr = "docker"
			testRunner = "docker compose run --rm test"
			buildTool = "docker build -t app ."
			rootPath = searchDir
			break
		}

		parent := filepath.Dir(searchDir)
		if parent == searchDir {
			break
		}
		searchDir = parent
	}

	gitBranch := appcontext.DetectGitBranch(curr)
	hasGit := gitBranch != ""

	if rootPath == "" {
		rootPath = curr
		if hasGit {
			eco = EcosystemGenericGit
		}
	}

	return ProjectInfo{
		RootPath:       rootPath,
		Ecosystem:      eco,
		PackageManager: pkgMgr,
		TestRunner:     testRunner,
		BuildTool:      buildTool,
		GitRepo:        hasGit,
		GitBranch:      gitBranch,
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
