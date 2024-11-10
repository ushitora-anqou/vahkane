package discord

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Client struct {
	applicationID, token string
	httpClient           *http.Client
}

func NewClient(applicationID, token string) *Client {
	return &Client{
		applicationID: applicationID,
		token:         token,
		httpClient:    &http.Client{},
	}
}

func (c *Client) RegisterGuildCommands(
	ctx context.Context,
	guildID string,
	commandsJSON string,
) error {
	endpoint := fmt.Sprintf(
		"https://discord.com/api/v10/applications/%s/guilds/%s/commands",
		c.applicationID,
		guildID,
	)

	req, err := http.NewRequestWithContext(
		ctx, "POST", endpoint, strings.NewReader(commandsJSON))
	if err != nil {
		return err
	}
	req.Header.Add("user-agent", "vahkane")
	req.Header.Add("content-type", "application/json")
	req.Header.Add("authorization", "Bot "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("fail to register: %s: %s", resp.Status, string(body))
	}

	return nil
}
