// manifest/loader.go
package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Format represents the manifest file format
type Format string

const (
	FormatYAML    Format = "yaml"
	FormatJSON    Format = "json"
	FormatUnknown Format = "unknown"
)

// LoadFromFile loads manifest from a file (auto-detects format)
func (r *Registry) LoadFromFile(filepath string) error {
	data, err := os.ReadFile(filepath)
	if err != nil {
		if os.IsNotExist(err) {
			return NewFileNotFoundError(filepath)
		}
		return NewFileReadError(filepath, err)
	}

	format := DetectFormat(filepath, data)
	return r.LoadFromBytes(data, format)
}

// LoadFromYAML loads manifest from YAML data
func (r *Registry) LoadFromYAML(data []byte) error {
	return r.LoadFromBytes(data, FormatYAML)
}

// LoadFromJSON loads manifest from JSON data
func (r *Registry) LoadFromJSON(data []byte) error {
	return r.LoadFromBytes(data, FormatJSON)
}

// LoadFromBytes loads manifest from bytes with specified format
func (r *Registry) LoadFromBytes(data []byte, format Format) error {
	manifest, err := ParseManifest(data, format)
	if err != nil {
		return err
	}

	if err := ValidateManifest(manifest); err != nil {
		return err
	}

	return r.Load(manifest)
}

// Reload reloads manifest from the same file
func (r *Registry) Reload(filepath string) error {
	return r.LoadFromFile(filepath)
}

// LoadManifest loads manifest from a file (convenience function)
func LoadManifest(filepath string) (*Manifest, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, NewFileNotFoundError(filepath)
		}
		return nil, NewFileReadError(filepath, err)
	}

	format := DetectFormat(filepath, data)
	manifest, err := ParseManifest(data, format)
	if err != nil {
		return nil, err
	}

	if err := ValidateManifest(manifest); err != nil {
		return nil, err
	}

	return manifest, nil
}

// ParseManifest parses manifest from bytes with specified format
func ParseManifest(data []byte, format Format) (*Manifest, error) {
	var manifest Manifest

	switch format {
	case FormatYAML:
		if err := yaml.Unmarshal(data, &manifest); err != nil {
			return nil, NewInvalidYAMLError(err)
		}
	case FormatJSON:
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, NewInvalidJSONError(err)
		}
	default:
		// Try JSON first, then YAML
		if err := json.Unmarshal(data, &manifest); err != nil {
			if err := yaml.Unmarshal(data, &manifest); err != nil {
				return nil, NewInvalidFormatError(format)
			}
		}
	}

	return &manifest, nil
}

// ParseManifestYAML parses YAML manifest
func ParseManifestYAML(data []byte) (*Manifest, error) {
	return ParseManifest(data, FormatYAML)
}

// ParseManifestJSON parses JSON manifest
func ParseManifestJSON(data []byte) (*Manifest, error) {
	return ParseManifest(data, FormatJSON)
}

// DetectFormat detects the format from file extension or content
func DetectFormat(filepath string, data []byte) Format {
	// Try by extension first
	ext := strings.ToLower(filepath)
	if strings.HasSuffix(ext, ".json") {
		return FormatJSON
	}
	if strings.HasSuffix(ext, ".yaml") || strings.HasSuffix(ext, ".yml") {
		return FormatYAML
	}

	// Try to detect from content
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return FormatJSON
	}

	// Default to YAML
	return FormatYAML
}

// GetFormatFromPath returns format based on file path
func GetFormatFromPath(path string) Format {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		return FormatJSON
	case ".yaml", ".yml":
		return FormatYAML
	default:
		return FormatUnknown
	}
}

// ValidateManifest validates a manifest
func ValidateManifest(manifest *Manifest) error {
	if manifest.Version == "" {
		return NewMissingVersionError()
	}

	if len(manifest.Routes) == 0 {
		return NewMissingRoutesError()
	}

	// Track route names to detect duplicates
	routeNames := make(map[string]bool)
	validationErrors := make([]error, 0)

	// Validate each route
	for i, route := range manifest.Routes {
		if err := validateRoute(&route); err != nil {
			validationErrors = append(validationErrors, err)
		}

		// Check for duplicate route names
		if routeNames[route.Name] {
			validationErrors = append(validationErrors,
				NewDuplicateRouteNameError(route.Name))
		}
		routeNames[route.Name] = true

		// Validate route pattern can be compiled
		if _, err := compileRoutePattern(&manifest.Routes[i]); err != nil {
			validationErrors = append(validationErrors, err)
		}
	}

	// Validate fallback if present
	if manifest.Fallback != nil {
		if err := validateRoute(manifest.Fallback); err != nil {
			validationErrors = append(validationErrors, err)
		}
	}

	if len(validationErrors) > 0 {
		return NewMultipleValidationErrors(validationErrors)
	}

	return nil
}

// validateRoute validates a single route
func validateRoute(route *Route) error {
	if route.Pattern == "" {
		return NewMissingRoutePatternError(route.Name)
	}
	if route.Name == "" {
		return NewMissingRouteNameError(route.Pattern)
	}

	// Validate providers
	for _, provider := range route.Context.Providers {
		if err := validateProvider(&provider); err != nil {
			return err
		}
	}

	return nil
}

// validateProvider validates a single provider
func validateProvider(provider *Provider) error {
	if provider.Type == "" {
		return NewMissingProviderTypeError(provider.Name)
	}

	if provider.Name == "" {
		return NewMissingProviderNameError()
	}

	// Type-specific validation
	switch provider.Type {
	case "http":
		if provider.URL == "" {
			return NewMissingProviderURLError(provider.Name, provider.Type)
		}
	}

	return nil
}

// compileRoutePattern validates that a route pattern can be compiled
func compileRoutePattern(route *Route) (string, error) {
	// This is called during validation, but we don't store the result
	// The actual compilation happens in Registry.compileRoute
	pattern := route.Pattern

	// Basic pattern validation
	if !strings.HasPrefix(pattern, "/") && pattern != "" {
		return "", NewInvalidPatternError(pattern,
			NewValidationError("pattern must start with /"))
	}

	return pattern, nil
}

// SaveManifest saves manifest to a file with the specified format
func SaveManifest(manifest *Manifest, filepath string, format Format) error {
	var data []byte
	var err error

	switch format {
	case FormatJSON:
		data, err = json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return NewInvalidJSONError(err)
		}
	case FormatYAML:
		data, err = yaml.Marshal(manifest)
		if err != nil {
			return NewInvalidYAMLError(err)
		}
	default:
		// Auto-detect from file extension
		format = GetFormatFromPath(filepath)
		if format == FormatJSON {
			data, err = json.MarshalIndent(manifest, "", "  ")
			if err != nil {
				return NewInvalidJSONError(err)
			}
		} else {
			data, err = yaml.Marshal(manifest)
			if err != nil {
				return NewInvalidYAMLError(err)
			}
		}
	}

	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return NewFileWriteError(filepath, err)
	}

	return nil
}
