package email

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/mailgun/mailgun-go"
	"github.com/ntbosscher/gobase/env"
	"html/template"
	"io/ioutil"
	"mime"
	"net/http"
	"path"
	"strings"
)

var mailgunDomain string
var mailgunAPIKey string
var provider string
var postmarkAPIKey string
var postmarkStream string
var defaultFrom string

const mailgunProvider = "mailgun"
const postmarkProvider = "postmark"

func init() {
	provider = env.Optional("EMAIL_PROVIDER", mailgunProvider)
	defaultFrom = env.Require("DEFAULT_EMAIL_FROM")

	switch provider {
	case mailgunProvider:
		mailgunDomain = env.Require("MAILGUN_DOMAIN")
		mailgunAPIKey = env.Require("MAILGUN_API_KEY")
	case postmarkProvider:
		postmarkStream = env.Optional("POSTMARK_STREAM", "outbound")
		postmarkAPIKey = env.Require("POSTMARK_API_KEY")
	}

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

	// ContentType is the mime-type.
	// if not provided the mime-type will be auto-calculated using the Name's extension
	ContentType string
}

func SendHTML(to string, subject string, body string, attachments ...*Attachment) error {

	em := &Email{
		To:          []string{to},
		Subject:     subject,
		HTML:        body,
		Attachments: attachments,
	}

	return em.Send()
}

func Send(to string, subject string, body string) error {
	em := &Email{
		To:      []string{to},
		Subject: subject,
		Text:    body,
	}

	return em.Send()
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

	switch provider {
	case mailgunProvider:
		return e.sendMailgun()
	case postmarkProvider:
		return e.sendPostmark()
	default:
		return errors.New("invalid mail provider '" + provider + "'")
	}
}

type postmarkAttachment struct {
	Name        string
	Content     string
	ContentType string
}

func (e *Email) sendPostmark() error {
	if env.IsTesting {
		e.To = []string{"test@blackhole.postmarkapp.com"}
	}

	body := map[string]interface{}{
		"To":            strings.Join(e.To, ", "),
		"From":          e.From,
		"Subject":       e.Subject,
		"MessageStream": postmarkStream,
	}

	if e.ReplyTo != "" {
		body["ReplyTo"] = e.ReplyTo
	}

	if e.HTML != "" {
		body["HtmlBody"] = e.HTML
	}

	if e.Text != "" {
		body["TextBody"] = e.Text
	}

	if len(e.Attachments) > 0 {
		list := []*postmarkAttachment{}

		for _, att := range e.Attachments {
			list = append(list, &postmarkAttachment{
				Name:        att.Name,
				Content:     base64.StdEncoding.EncodeToString(att.Value),
				ContentType: mime.TypeByExtension(path.Ext(att.Name)),
			})
		}

		body["Attachments"] = list
	}

	jsonBytes, err := json.Marshal(body)
	if err != nil {
		return err
	}

	rq, err := http.NewRequest("POST", "https://api.postmarkapp.com/email", bytes.NewReader(jsonBytes))
	if err != nil {
		return err
	}

	rq.Header.Set("X-Postmark-Server-Token", postmarkAPIKey)
	rq.Header.Set("Accept", "application/json")
	rq.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(rq)
	if err != nil {
		return err
	}

	defer res.Body.Close()

	resultJson, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	if res.StatusCode >= 400 {
		return errors.New("postmark failed to send email: " + string(resultJson))
	}

	return nil
}

func (e *Email) sendMailgun() error {
	mg := mailgun.NewMailgun(mailgunDomain, mailgunAPIKey)
	msg := mg.NewMessage(e.From, e.Subject, e.Text, e.To...)

	if env.IsTesting {
		msg.EnableTestMode()
	}

	if e.ReplyTo != "" {
		msg.SetReplyTo(e.ReplyTo)
	}

	if e.HTML != "" {
		msg.SetHtml(e.HTML)
	}

	for _, attachment := range e.Attachments {
		msg.AddBufferAttachment(attachment.Name, attachment.Value)
	}

	_, _, err := mg.Send(msg)
	return err
}
