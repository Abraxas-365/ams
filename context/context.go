package context

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/Abraxas-365/ams/pkg/ai/llm"
)

// FullContext contains all context for the agent
type FullContext struct {
	Route          RouteInfo        `json:"route"`
	User           *User            `json:"user,omitempty"`
	Frontend       *FrontendContext `json:"frontend,omitempty"`
	Backend        map[string]any   `json:"backend"`
	Instructions   string           `json:"instructions"`
	AvailableTools []string         `json:"available_tools"`
}

// RouteInfo contains information about the current route
type RouteInfo struct {
	Path   string            `json:"path"`
	Name   string            `json:"name"`
	Params map[string]string `json:"params"`
	Query  map[string]string `json:"query"`
}

// User represents the current user
type User struct {
	ID          string         `json:"id"`
	Email       string         `json:"email,omitempty"`
	Name        string         `json:"name,omitempty"`
	Permissions []string       `json:"permissions,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`

	// 🔐 User's authentication token (for tool authentication)
	Token string `json:"token,omitempty"`
}

// FrontendContext contains context from the frontend/widget
type FrontendContext struct {
	AnonymousID   string                `json:"anonymous_id,omitempty"` // ✅ Unique ID for anonymous users
	Accessibility *AccessibilityContext `json:"accessibility,omitempty"`
	Viewport      *ViewportInfo         `json:"viewport,omitempty"`
	CustomData    map[string]any        `json:"custom_data,omitempty"`
}

// AccessibilityContext contains the accessibility tree from the frontend
type AccessibilityContext struct {
	Title               string               `json:"title"`
	InteractiveElements []InteractiveElement `json:"interactive_elements"`
	Headings            []Heading            `json:"headings"`
}

// InteractiveElement represents an interactive element on the page
type InteractiveElement struct {
	Type  string `json:"type"`
	Label string `json:"label"`
	Value string `json:"value,omitempty"`
	ID    string `json:"id,omitempty"`
}

// Heading represents a heading on the page
type Heading struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
}

// ViewportInfo contains viewport information
type ViewportInfo struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// ToSystemMessage converts the full context to an LLM system message
func (fc *FullContext) ToSystemMessage() llm.Message {
	return llm.NewSystemMessage(fc.String())
}

// String renders the context as a formatted string for the LLM
func (fc *FullContext) String() string {
	var sb strings.Builder

	sb.WriteString("=== CURRENT PAGE CONTEXT ===\n\n")

	// Route info
	fmt.Fprintf(&sb, "Page: %s (%s)\n", fc.Route.Name, fc.Route.Path)
	if len(fc.Route.Params) > 0 {
		fmt.Fprintf(&sb, "Parameters: %v\n", fc.Route.Params)
	}
	if len(fc.Route.Query) > 0 {
		fmt.Fprintf(&sb, "Query: %v\n", fc.Route.Query)
	}
	sb.WriteString("\n")

	// Instructions
	if fc.Instructions != "" {
		sb.WriteString("=== YOUR INSTRUCTIONS ===\n")
		sb.WriteString(fc.Instructions)
		sb.WriteString("\n\n")
	}

	// Backend Data
	if len(fc.Backend) > 0 {
		sb.WriteString("=== BACKEND DATA ===\n\n")
		for key, value := range fc.Backend {
			fmt.Fprintf(&sb, "%s:\n", key)
			jsonData, _ := json.MarshalIndent(value, "", "  ")
			sb.WriteString(string(jsonData))
			sb.WriteString("\n\n")
		}
	}

	// Frontend Context
	if fc.Frontend != nil && fc.Frontend.Accessibility != nil {
		sb.WriteString("=== PAGE STRUCTURE ===\n")
		acc := fc.Frontend.Accessibility

		fmt.Fprintf(&sb, "Title: %s\n", acc.Title)

		if len(acc.Headings) > 0 {
			sb.WriteString("\nHeadings:\n")
			for _, h := range acc.Headings {
				fmt.Fprintf(&sb, "  H%d: %s\n", h.Level, h.Text)
			}
		}

		if len(acc.InteractiveElements) > 0 {
			sb.WriteString("\nInteractive Elements:\n")
			for _, el := range acc.InteractiveElements {
				fmt.Fprintf(&sb, "  - %s: %s", el.Type, el.Label)
				if el.Value != "" {
					fmt.Fprintf(&sb, " (value: %s)", el.Value)
				}
				sb.WriteString("\n")
			}
		}
		sb.WriteString("\n")
	}

	// Available tools
	if len(fc.AvailableTools) > 0 {
		sb.WriteString("=== AVAILABLE TOOLS ===\n")
		for _, tool := range fc.AvailableTools {
			fmt.Fprintf(&sb, "- %s\n", tool)
		}
		sb.WriteString("\n")
	}

	// User info
	if fc.User != nil {
		fmt.Fprintf(&sb, "User: %s", fc.User.Name)
		if fc.User.Email != "" {
			fmt.Fprintf(&sb, " (%s)", fc.User.Email)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// ToJSON converts context to JSON
func (fc *FullContext) ToJSON() ([]byte, error) {
	return json.MarshalIndent(fc, "", "  ")
}

// Helper methods

// IsAuthenticated checks if the user is authenticated
func (u *User) IsAuthenticated() bool {
	return u != nil && u.ID != ""
}

// IsAnonymous checks if the user is anonymous (has no real authentication)
func (u *User) IsAnonymous() bool {
	return u != nil && strings.HasPrefix(u.ID, "anon_")
}

// HasPermission checks if the user has a specific permission
func (u *User) HasPermission(permission string) bool {
	if u == nil {
		return false
	}
	return slices.Contains(u.Permissions, permission)
}

// GetAnonymousID safely gets the anonymous ID from frontend context
func (fc *FrontendContext) GetAnonymousID() string {
	if fc == nil {
		return ""
	}
	return fc.AnonymousID
}
