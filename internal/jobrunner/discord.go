package jobrunner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	discordMaxRetries = 3
	discordRetryDelay = 2 * time.Second
)

// SendDiscordNotification sends a message to a Discord webhook with retry logic.
func SendDiscordNotification(webhookURL, message string) error {
	if webhookURL == "" {
		return nil
	}

	payload := map[string]string{"content": message}
	jsonPayload, _ := json.Marshal(payload)

	var lastErr error
	retryDelay := discordRetryDelay

	for attempt := 1; attempt <= discordMaxRetries; attempt++ {
		req, err := http.NewRequest("POST", webhookURL, bytes.NewReader(jsonPayload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			lastErr = err
		} else {
			resp.Body.Close()
			if resp.StatusCode == 200 || resp.StatusCode == 204 {
				return nil
			}
			lastErr = fmt.Errorf("discord webhook failed with status %d", resp.StatusCode)
		}

		if attempt < discordMaxRetries {
			time.Sleep(retryDelay)
			retryDelay *= 2
		}
	}

	return lastErr
}
