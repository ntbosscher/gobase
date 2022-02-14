package email

import (
	"bytes"
	"errors"
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

const defaultLeftPadding = "37px"

type TemplateInput struct {
	// text that shows in the preview of a user's inbox
	PreHeader string

	// url or data-url
	Logo string

	Title          template.HTML
	Sections       []*Section
	ContactAddress []string
	FullWidth      bool
}

type Section struct {
	Type        string // button, big-button or html
	Width       string
	PaddingLeft string
	Button      ButtonDetails
	HTML        template.HTML
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
	content.PaddingLeft = "0px"

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

	for i, item := range cellContents {
		if i == 0 {
			item[0].PaddingLeft = defaultLeftPadding
		}

		list = append(list, item...)
	}

	list = append(list, &Section{
		Type: "flex-row-end",
	})

	return list
}

func SectionButtonVariant(text string, url string, variant string) *Section {
	return &Section{
		Type:        "button",
		PaddingLeft: defaultLeftPadding,
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

type Email struct {
	// To is a list of recipients to send to
	To []string

	// From is the email address this is 'sent from'
	// if left blank, DEFAULT_EMAIL_FROM will be used
	From string

	// ReplyTo defines the reply-to header, excluded if left blank
	ReplyTo string

	// Subject is the email's subject field
	Subject string

	// Text is the email's text version
	// if left blank, must specify HTML
	Text string

	// HTML is the email's html version
	// if left blank, this email will be sent as plain-text email
	HTML string

	// Attachments adds file attachments to the email
	Attachments []*Attachment
}

func (e *Email) Send() error {
	if len(e.To) == 0 {
		return errors.New("no email recipients specified")
	}

	if e.Text == "" && e.HTML == "" {
		return errors.New("missing email content")
	}

	if e.From == "" {
		e.From = defaultFrom
	}

	mg := mailgun.NewMailgun(mailgunDomain, mailgunAPIKey)
	msg := mg.NewMessage(e.From, e.Subject, e.Text, e.To...)

	if e.ReplyTo != "" {
		msg.SetReplyTo(e.ReplyTo)
	}

	if e.HTML != "" {
		msg.SetHtml(e.HTML)
	}

	for _, attachment := range e.Attachments {
		msg.AddBufferAttachment(attachment.Name, attachment.Value)
	}

	return send(mg, msg)
}
