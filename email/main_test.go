package email

import "testing"

func TestSend(t *testing.T) {

	em := &Email{
		To:      []string{"nate@productsonar.com"},
		ReplyTo: "support@productsonar.com",
		Subject: "Test 1-2-3",
		Text:    "hello world",
		Attachments: []*Attachment{{
			Name: "myfile.csv",
			Value: []byte(`test,test,1
test,test,2
test,test,3`),
		}},
	}

	if err := em.Send(); err != nil {
		t.Fatal(err)
	}

	t.Log("success")
}
