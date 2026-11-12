package plugins

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Capability defines a specific extension point supported by Cadence.
type Capability string

const (
	CapTranscriptionHook Capability = "transcription_hook"
	CapMeetingHook       Capability = "meeting_hook"
	CapAgentTool         Capability = "agent_tool"
	CapCommand           Capability = "command"
	CapPolishProfile     Capability = "polish_profile"
	CapStorage           Capability = "storage"
	CapNetwork           Capability = "network"
)

var validCapabilities = map[Capability]bool{
	CapTranscriptionHook: true,
	CapMeetingHook:       true,
	CapAgentTool:         true,
	CapCommand:           true,
	CapPolishProfile:     true,
	CapStorage:           true,
	CapNetwork:           true,
}

// Permissions specifies resources and access boundaries required by the plugin.
type Permissions struct {
	ReadTranscripts bool     `json:"read_transcripts" yaml:"read_transcripts"`
	ReadAudio       bool     `json:"read_audio" yaml:"read_audio"`
	Filesystem      []string `json:"filesystem,omitempty" yaml:"filesystem,omitempty"`
	Network         []string `json:"network,omitempty" yaml:"network,omitempty"`
}

// ToolDefinition defines an agent tool exposed by the plugin to Cadence's autonomous agent.
type ToolDefinition struct {
	Name        string         `json:"name" yaml:"name"`
	Description string         `json:"description" yaml:"description"`
	Parameters  map[string]any `json:"parameters,omitempty" yaml:"parameters,omitempty"`
}

// CommandDefinition defines a voice or CLI command registered by the plugin.
type CommandDefinition struct {
	Name          string   `json:"name" yaml:"name"`
	Description   string   `json:"description" yaml:"description"`
	Aliases       []string `json:"aliases,omitempty" yaml:"aliases,omitempty"`
	VoiceTriggers []string `json:"voice_triggers,omitempty" yaml:"voice_triggers,omitempty"`
}

// ConfigField describes a configurable user preference or secret for the plugin.
type ConfigField struct {
	Name        string `json:"name" yaml:"name"`
	Type        string `json:"type" yaml:"type"` // "string", "int", "bool"
	Description string `json:"description" yaml:"description"`
	Default     any    `json:"default,omitempty" yaml:"default,omitempty"`
	Required    bool   `json:"required,omitempty" yaml:"required,omitempty"`
}

// Manifest represents the complete metadata and capability specification of a Cadence plugin.
type Manifest struct {
	ID                string              `json:"id" yaml:"id"`
	Name              string              `json:"name" yaml:"name"`
	Version           string              `json:"version" yaml:"version"`
	Description       string              `json:"description" yaml:"description"`
	Author            string              `json:"author,omitempty" yaml:"author,omitempty"`
	Homepage          string              `json:"homepage,omitempty" yaml:"homepage,omitempty"`
	License           string              `json:"license,omitempty" yaml:"license,omitempty"`
	MinCadenceVersion string              `json:"min_cadence_version,omitempty" yaml:"min_cadence_version,omitempty"`
	Entrypoint        string              `json:"entrypoint" yaml:"entrypoint"`
	Capabilities      []Capability        `json:"capabilities" yaml:"capabilities"`
	Permissions       Permissions         `json:"permissions" yaml:"permissions"`
	Tools             []ToolDefinition    `json:"tools,omitempty" yaml:"tools,omitempty"`
	Commands          []CommandDefinition `json:"commands,omitempty" yaml:"commands,omitempty"`
	ConfigSchema      []ConfigField       `json:"config_schema,omitempty" yaml:"config_schema,omitempty"`
}

var (
	idRegex  = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]+$`)
	semverRe = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)
)

// ValidateManifest checks that a manifest satisfies all specification requirements.
func ValidateManifest(m *Manifest) error {
	if m == nil {
		return errors.New("manifest cannot be nil")
	}

	// 1. ID validation
	m.ID = strings.TrimSpace(m.ID)
	if m.ID == "" {
		return errors.New("plugin manifest requires non-empty 'id'")
	}
	if !idRegex.MatchString(m.ID) {
		return fmt.Errorf("plugin 'id' %q contains invalid characters (allowed: alphanumeric, dashes, dots, underscores)", m.ID)
	}

	// 2. Name
	m.Name = strings.TrimSpace(m.Name)
	if m.Name == "" {
		return errors.New("plugin manifest requires non-empty 'name'")
	}

	// 3. Version
	m.Version = strings.TrimSpace(m.Version)
	if m.Version == "" {
		return errors.New("plugin manifest requires non-empty 'version'")
	}
	if !semverRe.MatchString(m.Version) {
		return fmt.Errorf("plugin 'version' %q is not a valid semantic version (expected X.Y.Z)", m.Version)
	}

	// 4. Entrypoint
	m.Entrypoint = strings.TrimSpace(m.Entrypoint)
	if m.Entrypoint == "" {
		return errors.New("plugin manifest requires non-empty 'entrypoint'")
	}

	// 5. Capabilities
	for _, cap := range m.Capabilities {
		if !validCapabilities[cap] {
			return fmt.Errorf("unrecognized capability %q in plugin %s", cap, m.ID)
		}
	}

	// 6. Tools
	for _, t := range m.Tools {
		if strings.TrimSpace(t.Name) == "" {
			return fmt.Errorf("tool definition in plugin %s has empty name", m.ID)
		}
	}

	// 7. Commands
	for _, c := range m.Commands {
		if strings.TrimSpace(c.Name) == "" {
			return fmt.Errorf("command definition in plugin %s has empty name", m.ID)
		}
	}

	return nil
}

// ParseManifest parses manifest data from JSON or YAML into a validated Manifest.
func ParseManifest(data []byte) (*Manifest, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, errors.New("empty manifest data")
	}

	var m Manifest
	// If it starts with '{', parse as JSON
	if trimmed[0] == '{' {
		if err := json.Unmarshal(trimmed, &m); err != nil {
			return nil, fmt.Errorf("parse json manifest: %w", err)
		}
	} else {
		// Parse YAML
		if err := parseYAMLManifest(trimmed, &m); err != nil {
			return nil, fmt.Errorf("parse yaml manifest: %w", err)
		}
	}

	if err := ValidateManifest(&m); err != nil {
		return nil, fmt.Errorf("invalid plugin manifest: %w", err)
	}
	return &m, nil
}

// LoadManifest reads and validates a plugin manifest from a file path or directory.
func LoadManifest(path string) (*Manifest, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	filePath := path
	if fi.IsDir() {
		candidates := []string{
			filepath.Join(path, "plugin.yaml"),
			filepath.Join(path, "plugin.yml"),
			filepath.Join(path, "cadence-plugin.yaml"),
			filepath.Join(path, "cadence-plugin.yml"),
			filepath.Join(path, "plugin.json"),
		}
		found := false
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				filePath = c
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("no plugin manifest (plugin.yaml, plugin.json) found in directory %s", path)
		}
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read plugin manifest: %w", err)
	}

	return ParseManifest(data)
}

// GenerateManifestTemplate generates a starter manifest scaffold.
func GenerateManifestTemplate(id, name, description string) *Manifest {
	if id == "" {
		id = "com.example.my-plugin"
	}
	if name == "" {
		name = "My Cadence Plugin"
	}
	if description == "" {
		description = "A custom extension for Cadence dictation and agent workflows."
	}

	return &Manifest{
		ID:                id,
		Name:              name,
		Version:           "0.1.0",
		Description:       description,
		Author:            "Author Name <author@example.com>",
		License:           "MIT",
		MinCadenceVersion: ">= 0.1.0",
		Entrypoint:        "./bin/plugin",
		Capabilities: []Capability{
			CapTranscriptionHook,
			CapAgentTool,
		},
		Permissions: Permissions{
			ReadTranscripts: true,
			Filesystem:      []string{"."},
		},
		Tools: []ToolDefinition{
			{
				Name:        "sample_tool",
				Description: "Demonstrates a custom tool callable by the Cadence agent.",
				Parameters: map[string]any{
					"query": "string parameter description",
				},
			},
		},
		Commands: []CommandDefinition{
			{
				Name:          "sample-command",
				Description:   "Executes a custom voice command.",
				VoiceTriggers: []string{"run sample command"},
			},
		},
	}
}

// SerializeYAML exports a Manifest into standard clean YAML format.
func (m *Manifest) SerializeYAML() ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(fmt.Sprintf("id: %s\n", m.ID))
	b.WriteString(fmt.Sprintf("name: %s\n", m.Name))
	b.WriteString(fmt.Sprintf("version: %s\n", m.Version))
	b.WriteString(fmt.Sprintf("description: %s\n", m.Description))
	if m.Author != "" {
		b.WriteString(fmt.Sprintf("author: %s\n", m.Author))
	}
	if m.License != "" {
		b.WriteString(fmt.Sprintf("license: %s\n", m.License))
	}
	if m.MinCadenceVersion != "" {
		b.WriteString(fmt.Sprintf("min_cadence_version: %s\n", m.MinCadenceVersion))
	}
	b.WriteString(fmt.Sprintf("entrypoint: %s\n", m.Entrypoint))

	if len(m.Capabilities) > 0 {
		b.WriteString("capabilities:\n")
		for _, c := range m.Capabilities {
			b.WriteString(fmt.Sprintf("  - %s\n", c))
		}
	}

	b.WriteString("permissions:\n")
	b.WriteString(fmt.Sprintf("  read_transcripts: %v\n", m.Permissions.ReadTranscripts))
	b.WriteString(fmt.Sprintf("  read_audio: %v\n", m.Permissions.ReadAudio))
	if len(m.Permissions.Filesystem) > 0 {
		b.WriteString("  filesystem:\n")
		for _, p := range m.Permissions.Filesystem {
			b.WriteString(fmt.Sprintf("    - %s\n", p))
		}
	}
	if len(m.Permissions.Network) > 0 {
		b.WriteString("  network:\n")
		for _, net := range m.Permissions.Network {
			b.WriteString(fmt.Sprintf("    - %s\n", net))
		}
	}

	if len(m.Tools) > 0 {
		b.WriteString("tools:\n")
		for _, t := range m.Tools {
			b.WriteString(fmt.Sprintf("  - name: %s\n", t.Name))
			b.WriteString(fmt.Sprintf("    description: %s\n", t.Description))
		}
	}

	if len(m.Commands) > 0 {
		b.WriteString("commands:\n")
		for _, c := range m.Commands {
			b.WriteString(fmt.Sprintf("  - name: %s\n", c.Name))
			b.WriteString(fmt.Sprintf("    description: %s\n", c.Description))
			if len(c.VoiceTriggers) > 0 {
				b.WriteString("    voice_triggers:\n")
				for _, vt := range c.VoiceTriggers {
					b.WriteString(fmt.Sprintf("      - %s\n", vt))
				}
			}
		}
	}

	return b.Bytes(), nil
}

// parseYAMLManifest provides a clean, robust YAML parser for Manifest without external libraries.
func parseYAMLManifest(data []byte, m *Manifest) error {
	lines := strings.Split(string(data), "\n")
	var currentSection string
	var subSection string
	var currentItemMap map[string]string
	var currentTriggers []string
	var currentAliases []string

	flushItem := func() {
		if currentItemMap == nil {
			return
		}
		if currentSection == "tools" {
			m.Tools = append(m.Tools, ToolDefinition{
				Name:        currentItemMap["name"],
				Description: currentItemMap["description"],
			})
		} else if currentSection == "commands" {
			m.Commands = append(m.Commands, CommandDefinition{
				Name:          currentItemMap["name"],
				Description:   currentItemMap["description"],
				VoiceTriggers: currentTriggers,
				Aliases:       currentAliases,
			})
		}
		currentItemMap = nil
		currentTriggers = nil
		currentAliases = nil
		subSection = ""
	}

	for _, rawLine := range lines {
		// Strip comments
		line := rawLine
		if commentIdx := strings.Index(line, "#"); commentIdx >= 0 {
			line = line[:commentIdx]
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		indent := len(line) - len(strings.TrimLeft(line, " "))

		// Top-level key: value
		if indent == 0 && strings.Contains(trimmed, ":") {
			flushItem()
			subSection = ""
			parts := strings.SplitN(trimmed, ":", 2)
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			val = strings.Trim(val, `"'`)

			switch key {
			case "id":
				m.ID = val
			case "name":
				m.Name = val
			case "version":
				m.Version = val
			case "description":
				m.Description = val
			case "author":
				m.Author = val
			case "homepage":
				m.Homepage = val
			case "license":
				m.License = val
			case "min_cadence_version":
				m.MinCadenceVersion = val
			case "entrypoint":
				m.Entrypoint = val
			case "capabilities", "permissions", "tools", "commands", "config_schema":
				currentSection = key
			}
			continue
		}

		// List items
		if strings.HasPrefix(trimmed, "- ") {
			itemVal := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			itemVal = strings.Trim(itemVal, `"'`)

			if subSection == "voice_triggers" {
				currentTriggers = append(currentTriggers, itemVal)
				continue
			}
			if subSection == "aliases" {
				currentAliases = append(currentAliases, itemVal)
				continue
			}

			switch currentSection {
			case "capabilities":
				m.Capabilities = append(m.Capabilities, Capability(itemVal))
			case "filesystem":
				m.Permissions.Filesystem = append(m.Permissions.Filesystem, itemVal)
			case "network":
				m.Permissions.Network = append(m.Permissions.Network, itemVal)
			case "tools":
				flushItem()
				currentItemMap = make(map[string]string)
				if strings.Contains(itemVal, ":") {
					kv := strings.SplitN(itemVal, ":", 2)
					currentItemMap[strings.TrimSpace(kv[0])] = strings.Trim(strings.TrimSpace(kv[1]), `"'`)
				}
			case "commands":
				flushItem()
				currentItemMap = make(map[string]string)
				if strings.Contains(itemVal, ":") {
					kv := strings.SplitN(itemVal, ":", 2)
					currentItemMap[strings.TrimSpace(kv[0])] = strings.Trim(strings.TrimSpace(kv[1]), `"'`)
				}
			}
			continue
		}

		// Nested fields
		if strings.Contains(trimmed, ":") {
			parts := strings.SplitN(trimmed, ":", 2)
			subKey := strings.TrimSpace(parts[0])
			subVal := strings.TrimSpace(parts[1])
			subVal = strings.Trim(subVal, `"'`)

			if currentSection == "permissions" {
				if subKey == "read_transcripts" {
					m.Permissions.ReadTranscripts = (subVal == "true" || subVal == "yes" || subVal == "1")
				} else if subKey == "read_audio" {
					m.Permissions.ReadAudio = (subVal == "true" || subVal == "yes" || subVal == "1")
				} else if subKey == "filesystem" {
					currentSection = "filesystem"
				} else if subKey == "network" {
					currentSection = "network"
				}
			} else if subKey == "voice_triggers" {
				subSection = "voice_triggers"
			} else if subKey == "aliases" {
				subSection = "aliases"
			} else if currentItemMap != nil {
				subSection = ""
				currentItemMap[subKey] = subVal
			}
		}
	}
	flushItem()

	return nil
}
