package email

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"html/template"
	"log"
	"net/smtp"
	"strings"

	"github.com/jlb922/gosaas/internal/config"
	ses "github.com/sourcegraph/go-ses"
	"gopkg.in/gomail.v2"
)

type AmazonSES struct{}

type Gmail struct{}

var auth smtp.Auth

type Request struct {
	from    string
	to      []string
	subject string
	body    string
}

// Send uses Amazon SES to send an HTML email, it will convert body to text automatically
func (a AmazonSES) Send(toEmail, toName, fromEmail, fromName, subject, body, replyTo string) error {
	if len(toEmail) == 0 || strings.Index(toEmail, "@") == -1 {
		return fmt.Errorf("empty To email")
	}

	m := gomail.NewMessage()
	m.SetHeader("To", toEmail)
	if len(replyTo) > 0 {
		m.SetHeader("From", fromName+" <"+replyTo+">")
		m.SetHeader("Reply-To", replyTo)
	} else {
		m.SetHeader("From", fromName+" <"+fromEmail+">")
	}
	m.SetHeader("Subject", subject)
	m.AddAlternative("text/plain", stripHTML(body))
	m.SetBody("text/html", body)

	var b bytes.Buffer
	m.WriteTo(&b)

	res, err := ses.EnvConfig.SendRawEmail(b.Bytes())
	if err != nil {
		return err
	} else if len(res) == 0 {
		return errors.New("No email id returned by Amazon SES")
	}

	return nil
}

func newRequest(to []string, subject, body string) *Request {
	return &Request{
		to:      to,
		subject: subject,
		body:    body,
	}
}

func (r *Request) parseTemplate(templateFileName string, data interface{}) error {
	t, err := template.ParseFiles(templateFileName)
	if err != nil {
		return err
	}
	buf := new(bytes.Buffer)
	if err = t.Execute(buf, data); err != nil {
		return err
	}
	r.body = buf.String()
	return nil
}

// Send uses GMail SMTP to send an HTML email, it will convert body to text automatically
// body holds the link to the reset form
// Code based off https://medium.com/@dhanushgopinath/sending-html-emails-using-templates-in-golang-9e953ca32f3d
// TODO - use a package like https://github.com/matcornic/hermes
func (g Gmail) Send(toEmail, toName, fromEmail, fromName, subject, body, replyTo string) error {
	if len(toEmail) == 0 || strings.Index(toEmail, "@") == -1 {
		return fmt.Errorf("empty To email")
	}

	GMAIL_USERNAME := config.Current.EmailLogin
	GMAIL_PASSWORD := config.Current.EmailPassword
	auth = smtp.PlainAuth("", GMAIL_USERNAME, GMAIL_PASSWORD, "smtp.gmail.com")
	templateData := struct {
		Name string
		URL  string
	}{
		Name: ",",
		URL:  body,
	}
	fmt.Println(templateData.URL)
	r := newRequest([]string{toEmail}, subject, body)
	if err := r.parseTemplate("./templates/forgot_email.html", templateData); err == nil {
		mime := "MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\n\n"
		subject := "Subject: " + r.subject + "\n"
		to := "To: " + toEmail + "\n"
		msg := []byte(subject + to + mime + "\n" + r.body)
		addr := "smtp.gmail.com:587"
		if err := smtp.SendMail(addr, auth, fromEmail, r.to, msg); err != nil {
			log.Fatal(err)
		}
	} else {
		fmt.Println("Error Parsing Template", err)
	}
	return nil
}

// stripHTML returns a version of a string with HTML tags stripped
func stripHTML(s string) string {
	output := ""

	// if we have a full html page we only need the body
	startBody := strings.Index(s, "<body")
	if startBody > -1 {
		endBody := strings.Index(s, "</body>")
		// try to find the end of the <body tag
		for i := startBody; i < endBody; i++ {
			if s[i] == '>' {
				startBody = i
				break
			}
		}

		if startBody < endBody {
			s = s[startBody:endBody]
		}
	}

	// Shortcut strings with no tags in them
	if !strings.ContainsAny(s, "<>") {
		output = s
	} else {
		// Removing line feeds
		s = strings.Replace(s, "\n", "", -1)

		// Then replace line breaks with newlines, to preserve that formatting
		s = strings.Replace(s, "</h1>", "\n\n", -1)
		s = strings.Replace(s, "</h2>", "\n\n", -1)
		s = strings.Replace(s, "</h3>", "\n\n", -1)
		s = strings.Replace(s, "</h4>", "\n\n", -1)
		s = strings.Replace(s, "</h5>", "\n\n", -1)
		s = strings.Replace(s, "</h6>", "\n\n", -1)
		s = strings.Replace(s, "</p>", "\n", -1)
		s = strings.Replace(s, "<br>", "\n", -1)
		s = strings.Replace(s, "<br/>", "\n", -1)
		s = strings.Replace(s, "<br />", "\n", -1)

		// Walk through the string removing all tags
		b := bytes.NewBufferString("")
		inTag := false
		for _, r := range s {
			switch r {
			case '<':
				inTag = true
			case '>':
				inTag = false
			default:
				if !inTag {
					b.WriteRune(r)
				}
			}
		}
		output = b.String()
	}

	// Remove a few common harmless entities, to arrive at something more like plain text
	output = strings.Replace(output, "&#8216;", "'", -1)
	output = strings.Replace(output, "&#8217;", "'", -1)
	output = strings.Replace(output, "&#8220;", "\"", -1)
	output = strings.Replace(output, "&#8221;", "\"", -1)
	output = strings.Replace(output, "&nbsp;", " ", -1)
	output = strings.Replace(output, "&quot;", "\"", -1)
	output = strings.Replace(output, "&apos;", "'", -1)

	// Translate some entities into their plain text equivalent (for example accents, if encoded as entities)
	output = html.UnescapeString(output)

	// In case we have missed any tags above, escape the text - removes <, >, &, ' and ".
	output = template.HTMLEscapeString(output)

	// After processing, remove some harmless entities &, ' and " which are encoded by HTMLEscapeString
	output = strings.Replace(output, "&#34;", "\"", -1)
	output = strings.Replace(output, "&#39;", "'", -1)
	output = strings.Replace(output, "&amp; ", "& ", -1)     // NB space after
	output = strings.Replace(output, "&amp;amp; ", "& ", -1) // NB space after

	return output
}
