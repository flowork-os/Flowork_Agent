package tui

import (
	"context"
	"strings"

	"github.com/teetah2402/flowork/internal/integrations"
)

func (m *model) handleGitHubInstall() {
	gh, err := integrations.NewGitHubIntegration()
	if err != nil {
		m.appendLocal("GITHUB", "Error: "+err.Error())
		return
	}

	result, err := gh.StartOAuthFlow(context.Background())
	if err != nil {
		m.appendLocal("GITHUB", "Error: "+err.Error())
		return
	}
	m.appendLocal("GITHUB", result)
}

func (m *model) handlePRCommand(args []string) {
	gh, err := integrations.NewGitHubIntegration()
	if err != nil {
		m.appendLocal("PR", "Error: "+err.Error())
		return
	}

	title := "FLOWORK changes"
	if len(args) > 0 {
		title = strings.Join(args, " ")
	}

	result, err := gh.CreatePR(context.Background(), title, "Created by FLOWORK", "")
	if err != nil {
		m.appendLocal("PR", "Error: "+err.Error())
		return
	}
	m.appendLocal("PR", result)
}
