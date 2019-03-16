package mailgun

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/getfider/fider/app/pkg/bus"
	"github.com/getfider/fider/app/services/httpclient"

	"github.com/getfider/fider/app/pkg/email"
	"github.com/getfider/fider/app/pkg/env"
	"github.com/getfider/fider/app/pkg/errors"
	"github.com/getfider/fider/app/pkg/log"
)

var baseURL = "https://api.mailgun.net/v3/%s/messages"

//Sender is used to send emails
type Sender struct {
	logger log.Logger
	domain string
	apiKey string
}

//NewSender creates a new mailgun email sender
func NewSender(logger log.Logger, domain, apiKey string) *Sender {
	return &Sender{logger, domain, apiKey}
}

//Send an email
func (s *Sender) Send(ctx email.Context, templateName string, params email.Params, from string, to email.Recipient) error {
	return s.BatchSend(ctx, templateName, params, from, []email.Recipient{to})
}

// BatchSend an email to multiple recipients
func (s *Sender) BatchSend(ctx email.Context, templateName string, params email.Params, from string, to []email.Recipient) error {
	if len(to) == 0 {
		return nil
	}

	isBatch := len(to) > 1

	var message *email.Message
	if isBatch {
		// Replace recipient specific Go templates variables with Mailgun template variables
		for k := range to[0].Params {
			params[k] = fmt.Sprintf("%%recipient.%s%%", k)
		}
		message = email.RenderMessage(ctx, templateName, params)
	} else {
		message = email.RenderMessage(ctx, templateName, params.Merge(to[0].Params))
	}

	form := url.Values{}
	form.Add("from", email.NewRecipient(from, email.NoReply, email.Params{}).String())
	form.Add("h:Reply-To", email.NoReply)
	form.Add("subject", message.Subject)
	form.Add("html", message.Body)
	form.Add("o:tag", fmt.Sprintf("template:%s", templateName))
	if ctx.Tenant() != nil && !env.IsSingleHostMode() {
		form.Add("o:tag", fmt.Sprintf("tenant:%s", ctx.Tenant().Subdomain))
	}

	// Set Mailgun's var based on each recipient's variables
	recipientVariables := make(map[string]email.Params, 0)
	for _, r := range to {
		if r.Address != "" {
			if email.CanSendTo(r.Address) {
				form.Add("to", r.String())
				recipientVariables[r.Address] = r.Params
			} else {
				s.logger.Warnf("Skipping email to '@{Name} <@{Address}>'.", log.Props{
					"Name":    r.Name,
					"Address": r.Address,
				})
			}
		}
	}

	// If we skipped all recipients, just return
	if len(recipientVariables) == 0 {
		return nil
	}

	if isBatch {
		json, err := json.Marshal(recipientVariables)
		if err != nil {
			return errors.Wrap(err, "failed to marshal recipient variables")
		}

		form.Add("recipient-variables", string(json))
	}

	if isBatch {
		s.logger.Debugf("Sending email to @{CountRecipients} recipients with template @{TemplateName}.", log.Props{
			"CountRecipients": len(recipientVariables),
			"TemplateName":    templateName,
		})
	} else {
		s.logger.Debugf("Sending email to @{Address} with template @{TemplateName}.", log.Props{
			"Address":      to[0].Address,
			"TemplateName": templateName,
		})
	}

	url := fmt.Sprintf(baseURL, s.domain)

	cmd := &httpclient.Request{
		Method: "POST",
		URL:    url,
		Body:   strings.NewReader(form.Encode()),
		Headers: map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
		},
		BasicAuth: &httpclient.BasicAuth{
			User:     "api",
			Password: s.apiKey,
		},
	}
	err := bus.Dispatch(context.Background(), cmd)
	if err != nil {
		return errors.Wrap(err, "failed to send email with template %s", templateName)
	}
	s.logger.Debugf("Email sent with response code @{StatusCode}.", log.Props{
		"StatusCode": cmd.ResponseStatusCode,
	})
	return nil
}
