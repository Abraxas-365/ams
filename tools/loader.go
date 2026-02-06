// tools/loader.go
package tools

import (
	"fmt"

	"github.com/Abraxas-365/ams/manifest"
	"github.com/Abraxas-365/ams/pkg/ai/llm/toolx"
)

// ToolLoader creates LLM tools from manifest configuration
type ToolLoader struct{}

// NewToolLoader creates a new tool loader
func NewToolLoader() *ToolLoader {
	return &ToolLoader{}
}

// LoadFromRoute creates toolx.Toolx instances from route configuration
func (l *ToolLoader) LoadFromRoute(
	route *manifest.Route,
	workflowContext map[string]any,
	userToken string,
) ([]toolx.Toolx, error) {
	tools := make([]toolx.Toolx, 0, len(route.Tools))

	for _, toolDef := range route.Tools {
		tool, err := l.createTool(toolDef, workflowContext, userToken)
		if err != nil {
			return nil, fmt.Errorf("failed to create tool %s: %w", toolDef.Name, err)
		}
		tools = append(tools, tool)
	}

	return tools, nil
}

// createTool creates a specific tool implementation based on type
func (l *ToolLoader) createTool(
	toolDef manifest.Tool,
	workflowContext map[string]any,
	userToken string,
) (toolx.Toolx, error) {
	// Validate tool definition
	if err := l.validateTool(toolDef); err != nil {
		return nil, err
	}

	switch toolDef.Type {
	case "http":
		return NewHTTPTool(toolDef, workflowContext, userToken), nil
	default:
		return nil, NewUnsupportedToolTypeError(toolDef.Type)
	}
}

// validateTool validates a tool definition
func (l *ToolLoader) validateTool(tool manifest.Tool) error {
	if tool.Name == "" {
		return NewInvalidToolError("tool name is required")
	}

	if tool.Description == "" {
		return NewInvalidToolError("tool description is required")
	}

	if tool.Type == "" {
		return NewInvalidToolError("tool type is required")
	}

	// Type-specific validation
	switch tool.Type {
	case "http":
		if tool.Config.URL == "" {
			return NewInvalidToolError("URL is required for HTTP tools")
		}
		if tool.Config.Method == "" {
			tool.Config.Method = "GET" // Default
		}
	}

	// Validate parameters
	paramNames := make(map[string]bool)
	for _, param := range tool.Parameters {
		if param.Name == "" {
			return NewInvalidToolError("parameter name is required")
		}

		if paramNames[param.Name] {
			return NewInvalidToolError(fmt.Sprintf("duplicate parameter name: %s", param.Name))
		}
		paramNames[param.Name] = true

		if param.Type == "" {
			return NewInvalidToolError(fmt.Sprintf("parameter type is required for: %s", param.Name))
		}

		if param.Source == "" {
			return NewInvalidToolError(fmt.Sprintf("parameter source is required for: %s", param.Name))
		}

		if param.Source == "context" && param.ContextPath == "" {
			return NewInvalidToolError(fmt.Sprintf("context_path is required for context parameter: %s", param.Name))
		}
	}

	return nil
}
