package email

import (
	"bytes"
	"github.com/mailgun/mailgun-go"
	"github.com/ntbosscher/gobase/env"
	"log"
)

var mailgunDomain string
var mailgunAPIKey string
var defaultFrom string

func init() {
	mailgunDomain = env.Require("MAILGUN_DOMAIN")
	mailgunAPIKey = env.Require("MAILGUN_API_KEY")
	defaultFrom = env.Require("DEFAULT_EMAIL_FROM")
}

type TemplateInput struct {
	// text that shows in the preview of a user's inbox
	PreHeader string

	// url or data-url
	Logo string

	Title string
	Body  []string
}

func SendTemplate(to string, subject string, template *TemplateInput, attachments ...*Attachment) error {
	buf := bytes.NewBuffer([]byte{})
	err := DefaultTemplate.Execute(buf, template)
	if err != nil {
		return err
	}

	return SendHTML(to, subject, buf.String(), attachments...)
}

type Attachment struct {
	Name  string
	Value []byte
}

func SendHTML(to string, subject string, body string, attachments ...*Attachment) error {
	mg := mailgun.NewMailgun(mailgunDomain, mailgunAPIKey)
	msg := mg.NewMessage(defaultFrom, subject, body, to)
	msg.SetHtml(body)

	for _, attachment := range attachments {
		msg.AddBufferAttachment(attachment.Name, attachment.Value)
	}

	return send(mg, msg)
}

func Send(to string, subject string, body string) error {
	mg := mailgun.NewMailgun(mailgunDomain, mailgunAPIKey)
	msg := mg.NewMessage(defaultFrom, subject, body, to)

	return send(mg, msg)
}

func send(mg *mailgun.MailgunImpl, msg *mailgun.Message) error {
	if env.IsTesting {
		msg.EnableTestMode()
	}

	_, _, err := mg.Send(msg)
	if err != nil {
		log.Println(err)
	}

	return err
}
