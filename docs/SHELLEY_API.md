# Shelley API Documentation

The news-app uses the local Shelley API (available on exe.dev VMs) to run AI-powered news search jobs.

## Base URL

```
http://localhost:9999
```

The Shelley API runs locally on exe.dev VMs and provides access to various AI models.

## Authentication

All requests require the `X-Exedev-Userid` header. POST requests also require `X-Shelley-Request: 1` for CSRF protection.

## Endpoints

### Create Conversation

```
POST /api/conversations/new
```

**Headers:**
- `Content-Type: application/json`
- `X-Exedev-Userid: <user-id>`
- `X-Shelley-Request: 1`

**Request Body:**
```json
{
  "message": "Your prompt here",
  "model": "claude-sonnet-4.5"
}
```

**Available Models:**
- `claude-opus-4.5` - Most capable, slowest
- `claude-sonnet-4.5` - Recommended for news retrieval (fast, capable)
- `claude-haiku-4.5` - Fastest, less capable
- `gpt-5.2-codex` - OpenAI model
- `qwen3-coder-fireworks` - Qwen model via Fireworks
- `glm-4.7-fireworks` - GLM model via Fireworks

**Response:**
```json
{
  "conversation_id": "cXXXXXXX",
  "status": "accepted"
}
```

### Get Conversation

```
GET /api/conversation/{conversation_id}
```

**Headers:**
- `X-Exedev-Userid: <user-id>`

**Response:**
```json
{
  "conversation": {
    "conversation_id": "cXXXXXXX",
    "slug": "auto-generated-slug",
    "model": "claude-sonnet-4.5",
    "created_at": "2026-01-27T16:51:48Z",
    "updated_at": "2026-01-27T16:51:50Z",
    "archived": false
  },
  "messages": [
    {
      "message_id": "uuid",
      "sequence_id": 1,
      "type": "system",
      "llm_data": "...",
      "end_of_turn": false
    },
    {
      "message_id": "uuid", 
      "sequence_id": 2,
      "type": "user",
      "llm_data": "...",
      "end_of_turn": false
    },
    {
      "message_id": "uuid",
      "sequence_id": 3,
      "type": "agent",
      "llm_data": "{...JSON with Content array...}",
      "end_of_turn": true
    }
  ]
}
```

### List Conversations

```
GET /api/conversations
```

**Headers:**
- `X-Exedev-Userid: <user-id>`

**Response:** Array of conversation objects.

### Archive Conversation

```
POST /api/conversation/{conversation_id}/archive
```

**Headers:**
- `X-Exedev-Userid: <user-id>`
- `X-Shelley-Request: 1`

**Response:**
```json
{"status": "archived"}
```

### Delete Conversation

```
POST /api/conversation/{conversation_id}/delete
```

**Headers:**
- `X-Exedev-Userid: <user-id>`
- `X-Shelley-Request: 1`

**Response:**
```json
{"status": "deleted"}
```

Permanently deletes the conversation and all its messages.

## Message Types

- `system` - System prompt
- `user` - User message
- `agent` - Agent response
- `error` - Error message

## Checking Completion

Poll the conversation endpoint and check if the last `agent` message has `end_of_turn: true`:

```bash
END_OF_TURN=$(curl -s -H "X-Exedev-Userid: $USER" \
  "http://localhost:9999/api/conversation/$CONV_ID" | \
  jq -r '.messages | map(select(.type == "agent")) | last | .end_of_turn // false')
```

## Extracting Response Text

The agent's response is in the `llm_data` field as JSON. Extract the text content:

```bash
RESPONSE=$(curl -s -H "X-Exedev-Userid: $USER" \
  "http://localhost:9999/api/conversation/$CONV_ID" | \
  jq -r '.messages | map(select(.type == "agent")) | last | .llm_data' | \
  jq -r '.Content | map(select(.Type == 2)) | .[0].Text')
```

The `Content` array contains blocks with `Type: 2` for text content.

## Error Handling

If a conversation fails, the last message will have `type: "error"` with details in `llm_data`.

## Go Client Usage

The news-app includes a Go client for the Shelley API in `internal/jobrunner/shelley.go`:

```go
import "srv.exe.dev/internal/jobrunner"

client := jobrunner.NewShelleyClient("http://localhost:9999")

// Create conversation
convID, err := client.CreateConversation(ctx, jobID, "Find recent AI news")

// Poll until complete
for {
    conv, err := client.GetConversation(ctx, jobID, convID)
    if conv.IsComplete() {
        text := conv.GetLastAgentText()
        break
    }
    time.Sleep(10 * time.Second)
}

// Archive when done
client.ArchiveConversation(ctx, jobID, convID)
```

## Bash Example

```bash
# Create conversation
CONV_ID=$(curl -s -X POST "http://localhost:9999/api/conversations/new" \
  -H "Content-Type: application/json" \
  -H "X-Exedev-Userid: news-job-1" \
  -H "X-Shelley-Request: 1" \
  -d '{"message": "Find recent AI news", "model": "claude-sonnet-4.5"}' | \
  jq -r '.conversation_id')

# Poll until complete
while true; do
  END=$(curl -s -H "X-Exedev-Userid: news-job-1" \
    "http://localhost:9999/api/conversation/$CONV_ID" | \
    jq -r '.messages | map(select(.type == "agent")) | last | .end_of_turn // false')
  [ "$END" = "true" ] && break
  sleep 10
done

# Get response
curl -s -H "X-Exedev-Userid: news-job-1" \
  "http://localhost:9999/api/conversation/$CONV_ID" | \
  jq -r '.messages | map(select(.type == "agent")) | last | .llm_data' | \
  jq -r '.Content[0].Text'
```
