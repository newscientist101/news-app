package jobrunner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ShelleyClient is an HTTP client for the Shelley API.
type ShelleyClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewShelleyClient creates a new Shelley API client.
func NewShelleyClient(baseURL string) *ShelleyClient {
	return &ShelleyClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// jobUserID returns the exe.dev user ID header value for a job.
func jobUserID(jobID int64) string {
	return fmt.Sprintf("news-job-%d", jobID)
}

// CreateConversation creates a new conversation with the given prompt.
func (c *ShelleyClient) CreateConversation(ctx context.Context, jobID int64, prompt string) (string, error) {
	return c.CreateConversationAs(ctx, jobUserID(jobID), prompt)
}

// CreateConversationAs creates a new conversation with a custom user ID.
func (c *ShelleyClient) CreateConversationAs(ctx context.Context, userID, prompt string) (string, error) {
	reqBody := map[string]string{
		"message": prompt,
		"model":   "claude-sonnet-4.5",
	}
	jsonBody, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/conversations/new", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Exedev-Userid", userID)
	req.Header.Set("X-Shelley-Request", "1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Accept 2xx status codes (200 OK, 201 Created, 202 Accepted)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		ConversationID string `json:"conversation_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if result.ConversationID == "" {
		return "", fmt.Errorf("empty conversation ID in response")
	}

	return result.ConversationID, nil
}

// Conversation represents a Shelley conversation.
type Conversation struct {
	Conversation struct {
		ConversationID string `json:"conversation_id"`
		Working        *bool  `json:"working"`
	} `json:"conversation"`
	Messages []Message `json:"messages"`
}

// Message represents a message in a conversation.
type Message struct {
	Type      string          `json:"type"`
	EndOfTurn bool            `json:"end_of_turn"`
	LLMData   json.RawMessage `json:"llm_data"`
}

// LLMData represents the parsed LLM response data.
type LLMData struct {
	Content []ContentBlock `json:"Content"`
}

// ContentBlock represents a content block in the LLM response.
type ContentBlock struct {
	Type int    `json:"Type"` // 2 = text
	Text string `json:"Text"`
}

// IsComplete returns true if the conversation has finished.
func (c *Conversation) IsComplete() bool {
	// Use the working field if available
	if c.Conversation.Working != nil {
		return !*c.Conversation.Working
	}
	// Fallback to checking last agent message
	for i := len(c.Messages) - 1; i >= 0; i-- {
		if c.Messages[i].Type == "agent" {
			return c.Messages[i].EndOfTurn
		}
	}
	return false
}

// GetLastAgentText returns the text content from the last agent message.
func (c *Conversation) GetLastAgentText() string {
	for i := len(c.Messages) - 1; i >= 0; i-- {
		if c.Messages[i].Type == "agent" {
			var data LLMData
			if err := json.Unmarshal(c.Messages[i].LLMData, &data); err != nil {
				continue
			}
			for _, block := range data.Content {
				if block.Type == 2 && block.Text != "" {
					return block.Text
				}
			}
		}
	}
	return ""
}

// GetConversation retrieves a conversation by ID.
func (c *ShelleyClient) GetConversation(ctx context.Context, jobID int64, convID string) (*Conversation, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/conversation/"+convID, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-Exedev-Userid", jobUserID(jobID))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error %d", resp.StatusCode)
	}

	var conv Conversation
	if err := json.NewDecoder(resp.Body).Decode(&conv); err != nil {
		return nil, err
	}

	return &conv, nil
}

// DeleteConversation deletes/cancels a conversation.
func (c *ShelleyClient) DeleteConversation(ctx context.Context, jobID int64, convID string) error {
	return c.deleteConversationAs(ctx, jobUserID(jobID), convID)
}

// DeleteConversationAsCleanup deletes a conversation using the cleanup user ID.
func (c *ShelleyClient) DeleteConversationAsCleanup(ctx context.Context, convID string) error {
	return c.deleteConversationAs(ctx, "cleanup", convID)
}

func (c *ShelleyClient) deleteConversationAs(ctx context.Context, userID, convID string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", c.baseURL+"/api/conversation/"+convID, nil)
	if err != nil {
		return err
	}

	req.Header.Set("X-Exedev-Userid", userID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	return nil
}

// ArchiveConversation archives a conversation.
func (c *ShelleyClient) ArchiveConversation(ctx context.Context, jobID int64, convID string) error {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/conversation/"+convID+"/archive", nil)
	if err != nil {
		return err
	}

	req.Header.Set("X-Exedev-Userid", jobUserID(jobID))
	req.Header.Set("X-Shelley-Request", "1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	return nil
}

// ListSubagents returns conversation IDs of subagents for a parent conversation.
func (c *ShelleyClient) ListSubagents(ctx context.Context, jobID int64, parentConvID string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/conversations", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-Exedev-Userid", jobUserID(jobID))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var conversations []struct {
		ConversationID       string `json:"conversation_id"`
		ParentConversationID string `json:"parent_conversation_id"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&conversations); err != nil {
		return nil, err
	}

	var subagents []string
	for _, conv := range conversations {
		if conv.ParentConversationID == parentConvID {
			subagents = append(subagents, conv.ConversationID)
		}
	}

	return subagents, nil
}
