package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

//go:generate ../../bin/mockgen -source=$GOFILE -package=$GOPACKAGE -destination=mock_$GOFILE

type Client interface {
	SendFollowupMessage(ctx context.Context, interactionToken, message string) error
	GetGuildCommands(ctx context.Context, guildID string) ([]map[string]interface{}, error)
	RegisterGuildCommand(ctx context.Context, guildID, commandsJSON string) error
	DeleteGuildCommand(ctx context.Context, guildID, commandID string) error
}

type RealClient struct {
	applicationID, token string
	httpClient           *http.Client
}

func NewRealClient(applicationID, token string) Client {
	return &RealClient{
		applicationID: applicationID,
		token:         token,
		httpClient:    &http.Client{},
	}
}

func (c *RealClient) sendRequest(req *http.Request) ([]byte, error) {
	req.Header.Add("user-agent", "vahkane")
	req.Header.Add("content-type", "application/json")
	req.Header.Add("authorization", "Bot "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fail to send request: %s: %s", resp.Status, body)
	}

	return body, nil
}

func (c *RealClient) SendFollowupMessage(
	ctx context.Context,
	interactionToken, message string,
) error {
	endpoint := fmt.Sprintf(
		"https://discord.com/api/v10/webhooks/%s/%s",
		c.applicationID,
		interactionToken,
	)

	body, err := json.Marshal(map[string]interface{}{
		"content": message,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}

	_, err = c.sendRequest(req)
	return err
}

func (c *RealClient) GetGuildCommands(
	ctx context.Context,
	guildID string,
) ([]map[string]interface{}, error) {
	// cf. https://discord.com/developers/docs/interactions/application-commands#create-guild-application-command

	endpoint := fmt.Sprintf(
		"https://discord.com/api/v10/applications/%s/guilds/%s/commands",
		c.applicationID,
		guildID,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, strings.NewReader(""))
	if err != nil {
		return nil, err
	}

	body, err := c.sendRequest(req)
	if err != nil {
		return nil, err
	}

	parsedBody := []map[string]interface{}{}
	if err := json.Unmarshal(body, &parsedBody); err != nil {
		return nil, err
	}

	return parsedBody, nil
}

func (c *RealClient) RegisterGuildCommand(
	ctx context.Context,
	guildID string,
	commandsJSON string,
) error {
	// cf. https://discord.com/developers/docs/interactions/application-commands#create-guild-application-command

	endpoint := fmt.Sprintf(
		"https://discord.com/api/v10/applications/%s/guilds/%s/commands",
		c.applicationID,
		guildID,
	)

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(commandsJSON))
	if err != nil {
		return err
	}

	_, err = c.sendRequest(req)
	return err
}

func (c *RealClient) DeleteGuildCommand(
	ctx context.Context,
	guildID, commandID string,
) error {
	endpoint := fmt.Sprintf(
		"https://discord.com/api/v10/applications/%s/guilds/%s/commands/%s",
		c.applicationID,
		guildID,
		commandID,
	)

	req, err := http.NewRequestWithContext(ctx, "DELETE", endpoint, strings.NewReader(""))
	if err != nil {
		return err
	}

	_, err = c.sendRequest(req)
	return err
}
