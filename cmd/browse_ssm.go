package cmd

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/syasika/miniaws/internal/awsclient"
	"github.com/syasika/miniaws/internal/ssmops"
)

func fetchParams(ctx context.Context, cfg aws.Config, endpoint, requestToken string) tea.Msg {
	client := awsclient.NewSSMClient(cfg, endpoint)
	var tok *string
	if requestToken != "" {
		tok = &requestToken
	}
	page, err := ssmops.ListParameters(ctx, client, tok, 20)
	if err != nil {
		if ssmops.IsConnectionErr(err) {
			return paramsErrMsg{err: "Cannot reach ministack — is the container running?"}
		}
		return paramsErrMsg{err: fmt.Sprintf("SSM error: %v", err)}
	}
	params := make([]ssmParam, len(page.Parameters))
	for i, p := range page.Parameters {
		params[i] = ssmParam{Name: p.Name, Type: p.Type, Version: p.Version}
	}
	var nt string
	if page.NextToken != nil {
		nt = *page.NextToken
	}
	label := fmt.Sprintf("(%d)", len(params))
	if nt != "" {
		label += " [ more ]"
	}
	return paramsMsg{params: params, requestToken: requestToken, nextToken: nt, label: label}
}

func fetchParamValue(ctx context.Context, cfg aws.Config, endpoint, name string) tea.Msg {
	client := awsclient.NewSSMClient(cfg, endpoint)
	p, err := ssmops.GetParameter(ctx, client, name)
	if err != nil {
		return paramValueMsg{desc: fmt.Sprintf("Get failed: %v", err)}
	}
	return paramValueMsg{desc: fmt.Sprintf("%s = %s  (%s, v%d)", p.Name, p.Value, p.Type, p.Version)}
}

func doDeleteParam(ctx context.Context, cfg aws.Config, endpoint, name string) tea.Msg {
	client := awsclient.NewSSMClient(cfg, endpoint)
	if err := ssmops.DeleteParameter(ctx, client, name); err != nil {
		return resultMsg{desc: fmt.Sprintf("Delete failed: %v", err)}
	}
	return resultMsg{desc: fmt.Sprintf("Deleted parameter '%s'", name), refresh: true}
}
