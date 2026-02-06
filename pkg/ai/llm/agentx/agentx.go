package agentx

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/Abraxas-365/ams/pkg/ai/llm"
	"github.com/Abraxas-365/ams/pkg/ai/llm/memoryx"
	"github.com/Abraxas-365/ams/pkg/ai/llm/toolx"
	"github.com/Abraxas-365/ams/pkg/logx"
)

// Agent represents an LLM-powered agent with memory and tool capabilities
type Agent struct {
	client             *llm.Client
	tools              *toolx.ToolxClient
	memory             memoryx.Memory
	options            []llm.Option
	maxAutoIterations  int // Max iterations with "auto" tool choice
	maxTotalIterations int // Hard limit to prevent infinite loops
}

// AgentOption configures an Agent
type AgentOption func(*Agent)

// WithOptions adds LLM options to the agent
func WithOptions(options ...llm.Option) AgentOption {
	return func(a *Agent) {
		a.options = append(a.options, options...)
	}
}

// WithTools adds tools to the agent
func WithTools(tools *toolx.ToolxClient) AgentOption {
	return func(a *Agent) {
		a.tools = tools
	}
}

// WithMaxAutoIterations sets the maximum number of "auto" tool choice iterations
func WithMaxAutoIterations(max int) AgentOption {
	return func(a *Agent) {
		a.maxAutoIterations = max
	}
}

// WithMaxTotalIterations sets the hard limit for total iterations
func WithMaxTotalIterations(max int) AgentOption {
	return func(a *Agent) {
		a.maxTotalIterations = max
	}
}

// New creates a new agent
func New(client llm.Client, memory memoryx.Memory, opts ...AgentOption) *Agent {
	agent := &Agent{
		client:             &client,
		memory:             memory,
		maxAutoIterations:  3,  // Default: 3 "auto" iterations
		maxTotalIterations: 10, // Hard limit for safety
	}

	for _, opt := range opts {
		opt(agent)
	}

	logx.WithFields(logx.Fields{
		"max_auto_iterations":  agent.maxAutoIterations,
		"max_total_iterations": agent.maxTotalIterations,
		"has_tools":            agent.tools != nil,
	}).Debug("Agent initialized")

	return agent
}

// Run processes a user message and returns the final response
func (a *Agent) Run(ctx context.Context, userInput string) (string, error) {
	logx.WithField("user_input", userInput).Info("Starting agent run")

	// Add user message to memory
	if err := a.memory.Add(llm.NewUserMessage(userInput)); err != nil {
		logx.WithError(err).Error("Failed to add user message to memory")
		return "", fmt.Errorf("failed to add user message: %w", err)
	}
	logx.Debug("User message added to memory")

	// Get messages from memory
	messages, err := a.memory.Messages()
	if err != nil {
		logx.WithError(err).Error("Failed to retrieve messages from memory")
		return "", fmt.Errorf("failed to retrieve messages: %w", err)
	}
	logx.WithField("message_count", len(messages)).Debug("Retrieved messages from memory")

	// Check if tools are available and add them as options if so
	options := a.options
	if a.tools != nil {
		// Convert tools to LLM-compatible format
		toolList := a.getToolsList()
		if len(toolList) > 0 {
			options = append(options, llm.WithTools(toolList))
			logx.WithField("tool_count", len(toolList)).Debug("Added tools to LLM options")
		}
	}

	// Get response from LLM
	logx.Debug("Calling LLM")
	response, err := a.client.Chat(ctx, messages, options...)
	if err != nil {
		logx.WithError(err).Error("LLM call failed")
		return "", fmt.Errorf("LLM error: %w", err)
	}

	logx.WithFields(logx.Fields{
		"has_content":    response.Message.Content != "",
		"has_tool_calls": len(response.Message.ToolCalls) > 0,
		"token_usage":    response.Usage,
	}).Debug("LLM response received")

	// Add the response to memory
	if err := a.memory.Add(response.Message); err != nil {
		logx.WithError(err).Error("Failed to add assistant response to memory")
		return "", fmt.Errorf("failed to add assistant response: %w", err)
	}

	// Check if the response contains tool calls
	if len(response.Message.ToolCalls) > 0 && a.tools != nil {
		logx.WithField("tool_call_count", len(response.Message.ToolCalls)).Info("Processing tool calls")
		return a.handleToolCalls(ctx, response.Message.ToolCalls)
	}

	logx.Info("Agent run completed successfully")
	return response.Message.Content, nil
}

// RunStream streams the agent's initial response
// Note: This doesn't handle tool calls in streaming mode
func (a *Agent) RunStream(ctx context.Context, userInput string) (llm.Stream, error) {
	logx.WithField("user_input", userInput).Info("Starting agent stream")

	// Add user message to memory
	if err := a.memory.Add(llm.NewUserMessage(userInput)); err != nil {
		logx.WithError(err).Error("Failed to add user message to memory")
		return nil, fmt.Errorf("failed to add user message: %w", err)
	}

	// Get messages from memory
	messages, err := a.memory.Messages()
	if err != nil {
		logx.WithError(err).Error("Failed to retrieve messages from memory")
		return nil, fmt.Errorf("failed to retrieve messages: %w", err)
	}

	// Check if tools are available and add them as options if so
	options := a.options
	if a.tools != nil {
		toolList := a.getToolsList()
		if len(toolList) > 0 {
			options = append(options, llm.WithTools(toolList))
			logx.WithField("tool_count", len(toolList)).Debug("Added tools to stream options")
		}
	}

	// Get streaming response
	logx.Debug("Initiating stream")
	return a.client.ChatStream(ctx, messages, options...)
}

// handleToolCalls processes tool calls and returns the final response
func (a *Agent) handleToolCalls(ctx context.Context, toolCalls []llm.ToolCall) (string, error) {
	return a.handleToolCallsWithLimit(ctx, toolCalls, 0)
}

// handleToolCallsWithLimit processes tool calls with iteration limit
func (a *Agent) handleToolCallsWithLimit(ctx context.Context, toolCalls []llm.ToolCall, iteration int) (string, error) {
	logx.WithFields(logx.Fields{
		"iteration":       iteration,
		"tool_call_count": len(toolCalls),
	}).Debug("Handling tool calls")

	// Hard limit check
	if iteration >= a.maxTotalIterations {
		logx.WithFields(logx.Fields{
			"iteration":            iteration,
			"max_total_iterations": a.maxTotalIterations,
		}).Warn("Maximum total iterations exceeded")
		return "", fmt.Errorf("maximum total iterations (%d) exceeded", a.maxTotalIterations)
	}

	// Process each tool call
	for i, tc := range toolCalls {
		logx.WithFields(logx.Fields{
			"tool_index": i,
			"tool_name":  tc.Function.Name,
			"tool_id":    tc.ID,
		}).Debug("Executing tool")

		// Call the tool
		toolResponse, err := a.tools.Call(ctx, tc)
		if err != nil {
			logx.WithFields(logx.Fields{
				"tool_name": tc.Function.Name,
				"tool_id":   tc.ID,
			}).WithError(err).Error("Tool execution failed")
			return "", fmt.Errorf("tool execution error: %w", err)
		}

		logx.WithFields(logx.Fields{
			"tool_name": tc.Function.Name,
			"tool_id":   tc.ID,
		}).Debug("Tool executed successfully")

		// Add tool response to memory
		if err := a.memory.Add(toolResponse); err != nil {
			logx.WithError(err).Error("Failed to add tool response to memory")
			return "", fmt.Errorf("failed to add tool response: %w", err)
		}
	}

	// Get messages from memory
	messages, err := a.memory.Messages()
	if err != nil {
		logx.WithError(err).Error("Failed to retrieve messages from memory")
		return "", fmt.Errorf("failed to retrieve messages: %w", err)
	}

	// Smart tool choice: "auto" for first maxAutoIterations, then "none"
	options := a.options
	toolChoice := "none"
	if a.tools != nil {
		toolList := a.getToolsList()
		if len(toolList) > 0 {
			options = append(options, llm.WithTools(toolList))

			if iteration < a.maxAutoIterations {
				// First N iterations: allow "auto" tool calling
				toolChoice = "auto"
				options = append(options, llm.WithToolChoice("auto"))
			} else {
				// After N iterations: force "none" to prevent more tool calls
				toolChoice = "none"
				options = append(options, llm.WithToolChoice("none"))
				logx.WithField("iteration", iteration).Warn("Forcing tool choice to 'none' due to iteration limit")
			}
		}
	}

	logx.WithFields(logx.Fields{
		"iteration":   iteration,
		"tool_choice": toolChoice,
	}).Debug("Calling LLM with tool results")

	// Get next response from LLM with tool results
	response, err := a.client.Chat(ctx, messages, options...)
	if err != nil {
		logx.WithError(err).Error("LLM call failed after tool execution")
		return "", fmt.Errorf("LLM error: %w", err)
	}

	logx.WithFields(logx.Fields{
		"has_content":    response.Message.Content != "",
		"has_tool_calls": len(response.Message.ToolCalls) > 0,
		"token_usage":    response.Usage,
	}).Debug("LLM response received after tool execution")

	// Add the response to memory
	if err := a.memory.Add(response.Message); err != nil {
		logx.WithError(err).Error("Failed to add assistant response to memory")
		return "", fmt.Errorf("failed to add assistant response: %w", err)
	}

	// Check if we have more tool calls to handle
	if len(response.Message.ToolCalls) > 0 {
		logx.WithField("iteration", iteration+1).Debug("More tool calls to process")
		return a.handleToolCallsWithLimit(ctx, response.Message.ToolCalls, iteration+1)
	}

	logx.WithField("iteration", iteration).Info("Tool call chain completed")
	return response.Message.Content, nil
}

// getToolsList converts the tools to LLM-compatible format
func (a *Agent) getToolsList() []llm.Tool {
	return a.tools.GetTools()
}

// ClearMemory resets the conversation but keeps the system prompt
func (a *Agent) ClearMemory() error {
	logx.Info("Clearing agent memory")
	err := a.memory.Clear()
	if err != nil {
		logx.WithError(err).Error("Failed to clear memory")
		return err
	}
	logx.Debug("Memory cleared successfully")
	return nil
}

// AddMessage adds a message to memory
func (a *Agent) AddMessage(message llm.Message) error {
	logx.WithField("role", message.Role).Debug("Adding message to memory")
	err := a.memory.Add(message)
	if err != nil {
		logx.WithError(err).Error("Failed to add message to memory")
	}
	return err
}

// Messages returns all messages in memory
func (a *Agent) Messages() ([]llm.Message, error) {
	messages, err := a.memory.Messages()
	if err != nil {
		logx.WithError(err).Error("Failed to retrieve messages")
		return nil, err
	}
	logx.WithField("message_count", len(messages)).Debug("Retrieved messages")
	return messages, nil
}

// StreamWithTools streams responses while handling tool calls
// This is a more advanced implementation that processes tool calls in streaming mode
func (a *Agent) StreamWithTools(ctx context.Context, userInput string, streamHandler func(chunk string)) error {
	logx.WithField("user_input", userInput).Info("Starting stream with tools")

	if err := a.memory.Add(llm.NewUserMessage(userInput)); err != nil {
		logx.WithError(err).Error("Failed to add user message to memory")
		return fmt.Errorf("failed to add user message: %w", err)
	}

	messages, err := a.memory.Messages()
	if err != nil {
		logx.WithError(err).Error("Failed to retrieve messages from memory")
		return fmt.Errorf("failed to retrieve messages: %w", err)
	}

	options := a.options
	if a.tools != nil {
		toolList := a.getToolsList()
		if len(toolList) > 0 {
			options = append(options, llm.WithTools(toolList))
			logx.WithField("tool_count", len(toolList)).Debug("Added tools to stream")
		}
	}

	// Initial streaming response
	logx.Debug("Starting stream")
	stream, err := a.client.ChatStream(ctx, messages, options...)
	if err != nil {
		logx.WithError(err).Error("Failed to start stream")
		return err
	}
	defer stream.Close()

	// Collect the full message and stream chunks to the handler
	var fullMessage llm.Message
	var responseContent string
	var toolCalls []llm.ToolCall
	chunkCount := 0

	for {
		chunk, err := stream.Next()
		if err != nil {
			// Check if it's the end of the stream
			if errors.Is(err, io.EOF) {
				logx.WithField("chunk_count", chunkCount).Debug("Stream ended")
				// Some implementations might return a final chunk with the error
				if chunk.Role != "" {
					fullMessage = chunk
				}
				break
			}
			// Any other error is returned
			logx.WithError(err).Error("Stream error")
			return err
		}

		chunkCount++

		// Accumulate content
		if chunk.Content != "" {
			responseContent += chunk.Content
			streamHandler(chunk.Content)
		}

		// Collect tool calls if present
		if len(chunk.ToolCalls) > 0 {
			toolCalls = chunk.ToolCalls
			logx.WithField("tool_call_count", len(chunk.ToolCalls)).Debug("Tool calls detected in stream")
		}
	}

	// If we don't have a full message yet, construct one
	if fullMessage.Role == "" {
		fullMessage = llm.Message{
			Role:      llm.RoleAssistant,
			Content:   responseContent,
			ToolCalls: toolCalls,
		}
	}

	logx.WithFields(logx.Fields{
		"content_length": len(responseContent),
		"tool_calls":     len(toolCalls),
	}).Debug("Stream completed, processing full message")

	// Add the full message to memory
	if err := a.memory.Add(fullMessage); err != nil {
		logx.WithError(err).Error("Failed to add assistant response to memory")
		return fmt.Errorf("failed to add assistant response: %w", err)
	}

	// Process tool calls if any
	if len(fullMessage.ToolCalls) > 0 && a.tools != nil {
		logx.Info("Processing tool calls from stream")
		streamHandler("\n[Processing tool calls...]\n")

		finalResponse, err := a.handleToolCalls(ctx, fullMessage.ToolCalls)
		if err != nil {
			logx.WithError(err).Error("Failed to handle tool calls")
			return err
		}

		streamHandler("\n[Final response after tool calls]\n" + finalResponse)
		logx.Info("Stream with tools completed successfully")
	}

	return nil
}

// RunConversation runs a complete conversation with multiple turns
func (a *Agent) RunConversation(ctx context.Context, userInputs []string) ([]string, error) {
	logx.WithField("turn_count", len(userInputs)).Info("Starting conversation")
	var responses []string

	for i, input := range userInputs {
		logx.WithFields(logx.Fields{
			"turn":  i + 1,
			"total": len(userInputs),
		}).Debug("Processing conversation turn")

		response, err := a.Run(ctx, input)
		if err != nil {
			logx.WithFields(logx.Fields{
				"turn":  i + 1,
				"total": len(userInputs),
			}).WithError(err).Error("Conversation turn failed")
			return responses, err
		}
		responses = append(responses, response)
	}

	logx.Info("Conversation completed successfully")
	return responses, nil
}

// EvaluateWithTools runs the agent with tools and returns detailed execution info
func (a *Agent) EvaluateWithTools(ctx context.Context, userInput string) (*AgentEvaluation, error) {
	logx.WithField("user_input", userInput).Info("Starting evaluation with tools")

	eval := &AgentEvaluation{
		UserInput: userInput,
		Steps:     []AgentStep{},
	}

	// Add user message to memory
	if err := a.memory.Add(llm.NewUserMessage(userInput)); err != nil {
		logx.WithError(err).Error("Failed to add user message to memory")
		return nil, fmt.Errorf("failed to add user message: %w", err)
	}

	// Start evaluation process
	evalStep := AgentStep{
		StepType: "initial",
	}

	// Get messages from memory
	messages, err := a.memory.Messages()
	if err != nil {
		logx.WithError(err).Error("Failed to retrieve messages from memory")
		return nil, fmt.Errorf("failed to retrieve messages: %w", err)
	}
	evalStep.InputMessages = messages

	// Check if tools are available and add them as options if so
	options := a.options
	if a.tools != nil {
		toolList := a.getToolsList()
		if len(toolList) > 0 {
			options = append(options, llm.WithTools(toolList))
			logx.WithField("tool_count", len(toolList)).Debug("Added tools for evaluation")
		}
	}

	// Get response from LLM
	logx.Debug("Getting initial LLM response for evaluation")
	response, err := a.client.Chat(ctx, messages, options...)
	if err != nil {
		logx.WithError(err).Error("LLM call failed during evaluation")
		return nil, fmt.Errorf("LLM error: %w", err)
	}

	evalStep.OutputMessage = response.Message
	evalStep.TokenUsage = response.Usage
	eval.Steps = append(eval.Steps, evalStep)

	logx.WithFields(logx.Fields{
		"has_tool_calls": len(response.Message.ToolCalls) > 0,
		"token_usage":    response.Usage,
	}).Debug("Initial evaluation step completed")

	// Add the response to memory
	if err := a.memory.Add(response.Message); err != nil {
		logx.WithError(err).Error("Failed to add assistant response to memory")
		return nil, fmt.Errorf("failed to add assistant response: %w", err)
	}

	// Check if the response contains tool calls
	if len(response.Message.ToolCalls) > 0 && a.tools != nil {
		logx.WithField("tool_call_count", len(response.Message.ToolCalls)).Info("Evaluating tool calls")
		result, steps, err := a.evaluateToolCalls(ctx, response.Message.ToolCalls)
		if err != nil {
			logx.WithError(err).Error("Tool call evaluation failed")
			return nil, err
		}

		eval.Steps = append(eval.Steps, steps...)
		eval.FinalResponse = result
	} else {
		eval.FinalResponse = response.Message.Content
	}

	logx.WithField("total_steps", len(eval.Steps)).Info("Evaluation completed successfully")
	return eval, nil
}

// evaluateToolCalls processes tool calls and records evaluation steps
func (a *Agent) evaluateToolCalls(ctx context.Context, toolCalls []llm.ToolCall) (string, []AgentStep, error) {
	return a.evaluateToolCallsWithLimit(ctx, toolCalls, 0)
}

// evaluateToolCallsWithLimit processes tool calls with iteration limit
func (a *Agent) evaluateToolCallsWithLimit(ctx context.Context, toolCalls []llm.ToolCall, iteration int) (string, []AgentStep, error) {
	var steps []AgentStep

	logx.WithFields(logx.Fields{
		"iteration":       iteration,
		"tool_call_count": len(toolCalls),
	}).Debug("Evaluating tool calls with limit")

	// Hard limit check
	if iteration >= a.maxTotalIterations {
		logx.WithFields(logx.Fields{
			"iteration":            iteration,
			"max_total_iterations": a.maxTotalIterations,
		}).Warn("Maximum total iterations exceeded during evaluation")
		return "", steps, fmt.Errorf("maximum total iterations (%d) exceeded", a.maxTotalIterations)
	}

	// Process each tool call
	toolStep := AgentStep{
		StepType:  "tool_execution",
		ToolCalls: toolCalls,
	}

	var toolResponses []llm.Message
	for i, tc := range toolCalls {
		logx.WithFields(logx.Fields{
			"tool_index": i,
			"tool_name":  tc.Function.Name,
			"tool_id":    tc.ID,
		}).Debug("Evaluating tool execution")

		// Call the tool
		toolResponse, err := a.tools.Call(ctx, tc)
		if err != nil {
			logx.WithFields(logx.Fields{
				"tool_name": tc.Function.Name,
				"tool_id":   tc.ID,
			}).WithError(err).Error("Tool execution failed during evaluation")
			return "", steps, fmt.Errorf("tool execution error: %w", err)
		}

		toolResponses = append(toolResponses, toolResponse)

		// Add tool response to memory
		if err := a.memory.Add(toolResponse); err != nil {
			logx.WithError(err).Error("Failed to add tool response to memory")
			return "", steps, fmt.Errorf("failed to add tool response: %w", err)
		}
	}

	toolStep.ToolResponses = toolResponses
	steps = append(steps, toolStep)

	// Get messages from memory
	messages, err := a.memory.Messages()
	if err != nil {
		logx.WithError(err).Error("Failed to retrieve messages from memory")
		return "", steps, fmt.Errorf("failed to retrieve messages: %w", err)
	}

	// Get next response from LLM with tool results
	responseStep := AgentStep{
		StepType:      "response",
		InputMessages: messages,
	}

	options := a.options
	toolChoice := "none"
	if a.tools != nil {
		toolList := a.getToolsList()
		if len(toolList) > 0 {
			options = append(options, llm.WithTools(toolList))

			if iteration < a.maxAutoIterations {
				// First N iterations: allow "auto" tool calling
				toolChoice = "auto"
				options = append(options, llm.WithToolChoice("auto"))
			} else {
				// After N iterations: force "none" to prevent more tool calls
				toolChoice = "none"
				options = append(options, llm.WithToolChoice("none"))
				logx.WithField("iteration", iteration).Warn("Forcing tool choice to 'none' during evaluation")
			}
		}
	}

	logx.WithFields(logx.Fields{
		"iteration":   iteration,
		"tool_choice": toolChoice,
	}).Debug("Getting LLM response with tool results")

	response, err := a.client.Chat(ctx, messages, options...)
	if err != nil {
		logx.WithError(err).Error("LLM call failed during evaluation")
		return "", steps, fmt.Errorf("LLM error: %w", err)
	}

	responseStep.OutputMessage = response.Message
	responseStep.TokenUsage = response.Usage
	steps = append(steps, responseStep)

	// Add the response to memory
	if err := a.memory.Add(response.Message); err != nil {
		logx.WithError(err).Error("Failed to add assistant response to memory")
		return "", steps, fmt.Errorf("failed to add assistant response: %w", err)
	}

	// Check if we have more tool calls to handle
	if len(response.Message.ToolCalls) > 0 {
		logx.WithField("iteration", iteration+1).Debug("More tool calls to evaluate")
		result, moreSteps, err := a.evaluateToolCallsWithLimit(ctx, response.Message.ToolCalls, iteration+1)
		if err != nil {
			return "", steps, err
		}

		steps = append(steps, moreSteps...)
		return result, steps, nil
	}

	logx.WithField("iteration", iteration).Info("Tool call evaluation chain completed")
	return response.Message.Content, steps, nil
}

// Types for evaluation

type AgentEvaluation struct {
	UserInput     string      `json:"user_input"`
	Steps         []AgentStep `json:"steps"`
	FinalResponse string      `json:"final_response"`
}

type AgentStep struct {
	StepType      string         `json:"step_type"`      // "initial", "tool_execution", "response"
	InputMessages []llm.Message  `json:"input_message"`  // Messages sent to the LLM
	OutputMessage llm.Message    `json:"output_message"` // Response from the LLM
	ToolCalls     []llm.ToolCall `json:"tool_calls"`     // Tool calls made
	ToolResponses []llm.Message  `json:"tool_responses"` // Responses from the tools
	TokenUsage    llm.Usage      `json:"token_usage"`    // Token usage information
}
