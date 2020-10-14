package email

import (
	"github.com/mailgun/mailgun-go"
	"github.com/ntbosscher/gobase/env"
	"log"
)

var mailgunDomain string
var mailgunAPIKey string

func init() {
	mailgunDomain = env.Require("MAILGUN_DOMAIN")
	mailgunAPIKey = env.Require("MAILGUN_API_KEY")
}

type Attachment struct {
	Name  string
	Value []byte
}

func SendHTML(to string, subject string, body string, attachments ...*Attachment) error {
	mg := mailgun.NewMailgun(mailgunDomain, mailgunAPIKey)
	msg := mg.NewMessage("noreply@ontariometalproducts.com", subject, body, to)
	msg.SetHtml(body)

	for _, attachment := range attachments {
		msg.AddBufferAttachment(attachment.Name, attachment.Value)
	}

	return send(mg, msg, to, subject, body)
}

func Send(to string, subject string, body string) error {
	mg := mailgun.NewMailgun(mailgunDomain, mailgunAPIKey)
	msg := mg.NewMessage("noreply@ontariometalproducts.com", subject, body, to)

	return send(mg, msg, to, subject, body)
}

func send(mg *mailgun.MailgunImpl, msg *mailgun.Message, to string, subject string, body string) error {
	if env.IsTesting {
		msg.EnableTestMode()
	}

	_, _, err := mg.Send(msg)
	if err != nil {
		log.Println(err)
	}

	return err
}
