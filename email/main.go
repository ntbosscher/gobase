package email

import (
	"bytes"
	"github.com/mailgun/mailgun-go"
	"github.com/ntbosscher/gobase/env"
	"html/template"
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

	Title          template.HTML
	Sections       []*Section
	ContactAddress []string
}

type Section struct {
	Type   string // button, big-button or html
	Width  string
	Button ButtonDetails
	HTML   template.HTML
}

type ButtonDetails struct {
	Text    string
	URL     string
	Variant string
}

func SectionHTML(html string) *Section {
	return &Section{
		Type: "html",
		HTML: template.HTML(html),
	}
}

func Combine(input ...[]*Section) []*Section {
	flattened := []*Section{}

	for _, item := range input {
		flattened = append(flattened, item...)
	}

	return flattened
}

func SectionRowCell(content *Section, width string) []*Section {
	return []*Section{
		{
			Type:  "flex-item-start",
			Width: width,
		},
		content,
		{
			Type: "flex-item-end",
		},
	}
}

func SectionRow(cellContents ...[]*Section) []*Section {
	list := []*Section{{
		Type: "flex-row-start",
	}}

	for _, item := range cellContents {
		list = append(list, item...)
	}

	list = append(list, &Section{
		Type: "flex-row-end",
	})

	return list
}

func SectionButtonVariant(text string, url string, variant string) *Section {
	return &Section{
		Type: "button",
		Button: ButtonDetails{
			Text:    text,
			URL:     url,
			Variant: variant,
		},
	}
}

func SectionButton(text string, url string) *Section {
	return SectionButtonVariant(text, url, "")
}

func SectionBigButton(text string, url string) *Section {
	return &Section{
		Type: "big-button",
		Button: ButtonDetails{
			Text: text,
			URL:  url,
		},
	}
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
