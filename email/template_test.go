package email

import (
	"os"
	"testing"
)

func TestTemplate(t *testing.T) {

	input := &TemplateInput{
		PreHeader: "pre-header-text",
		Logo:      "/logo.png",
		Title:     "Hello World",
		FullWidth: true,
		Sections: Combine([]*Section{
			SectionHTML(`hey everyone,
we're doing this cool thing we want to talk about... blah, blah, blah'`),
			SectionButton("Sign Up", "https://google.ca"),
		},
			SectionRow(
				SectionRowCell(SectionButton("Sign Up", "https://google.ca"), "150px"),
				SectionRowCell(SectionButtonVariant("Sign Up", "https://google.ca", "outlined"), "150px")),
		),
		ContactAddress: []string{
			"134 Sesamie St",
			"Vancouver BC, Canada",
			"N3L3S3",
		},
	}

	output, err := os.Create("./output.html")
	if err != nil {
		t.Fatal(err)
	}

	defer output.Close()

	err = DefaultTemplate.Execute(output, input)
	if err != nil {
		t.Fatal(err)
	}
}
