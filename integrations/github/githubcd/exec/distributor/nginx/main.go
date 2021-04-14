package nginx

import (
	"bufio"
	"bytes"
	"io"
	"log"
	"os/exec"
	"regexp"
	"strings"
)

type textBlock struct {
	value string
}

type nginxUpstreamRow interface{}

type nginxUpstreamBlock struct {
	name string
	rows []nginxUpstreamRow
}

type nginxBlock interface{}

type ServerRow struct {
	Name       string
	Parameters []string
	Status     string
}

type File []nginxBlock

func (f File) String() string {
	wr := strings.Builder{}
	for _, item := range f {
		switch v := item.(type) {
		case textBlock:
			wr.WriteString(v.value)
		case *nginxUpstreamBlock:
			wr.WriteString(v.String())
		}
	}

	return wr.String()
}

func (u nginxUpstreamBlock) String() string {
	wr := strings.Builder{}
	wr.WriteString("upstream ")
	wr.WriteString(u.name)
	wr.WriteString("{\n")

	for _, item := range u.rows {
		switch v := item.(type) {
		case textBlock:
			wr.WriteString("\t")
			wr.WriteString(v.value)
			wr.WriteString("\n")
		case *ServerRow:
			wr.WriteString("\t")
			parts := []string{"server", v.Name}
			parts = append(parts, v.Parameters...)
			if v.Status != "" {
				parts = append(parts, v.Status)
			}

			wr.WriteString(strings.Join(parts, " "))
			wr.WriteString(";\n")
		}
	}

	wr.WriteString("}")

	return wr.String()
}

func ParseBackends(fileContents []byte) (File, []*ServerRow, error) {
	tokenizer := bufio.NewScanner(bytes.NewReader(fileContents))
	upstream := regexp.MustCompile(`(?m)^.*upstream ([^\{]+){`)
	inBlock := false

	tokenizer.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {

		if atEOF && len(data) == 0 {
			return 0, nil, io.EOF
		}

		if inBlock {
			i := bytes.IndexRune(data, '}')
			if i == -1 {
				if atEOF {
					advance = len(data)
					token = data
					return
				}

				return
			}

			token = data[0:i]
			advance = i + 1
			inBlock = false
			return
		}

		indexes := upstream.FindIndex(data)
		if indexes == nil {
			if atEOF {
				advance = len(data)
				token = data
				return
			}

			return 0, nil, nil
		}

		if indexes[0] != 0 {
			// block prefix
			token = data[0:indexes[0]]
			advance = indexes[0]
			return
		}

		// backend block
		token = data[0:indexes[1]]
		advance = indexes[1]
		inBlock = true
		return
	})

	result := File{}
	var upstreamBlock *nginxUpstreamBlock
	var err error
	var servers []*ServerRow

	for tokenizer.Scan() {
		if upstreamBlock != nil {
			var serversTmp []*ServerRow
			upstreamBlock.rows, servers, err = parseUpstreamBlock(tokenizer.Text())
			if err != nil {
				return nil, nil, err
			}

			servers = append(servers, serversTmp...)
			upstreamBlock = nil
			continue
		}

		upstreamMatch := upstream.FindStringSubmatch(tokenizer.Text())
		if upstreamMatch != nil {
			upstreamBlock = &nginxUpstreamBlock{
				name: upstreamMatch[1],
				rows: []nginxUpstreamRow{},
			}
			result = append(result, upstreamBlock)
			continue
		}

		result = append(result, textBlock{value: tokenizer.Text()})
	}

	if err = tokenizer.Err(); err != nil {
		return nil, nil, err
	}

	return result, servers, nil
}

func parseUpstreamBlock(text string) (rows []nginxUpstreamRow, servers []*ServerRow, err error) {
	text = strings.Trim(text, " \n\r")
	byLine := bufio.NewScanner(strings.NewReader(text))
	byLine.Split(bufio.ScanLines)

	for byLine.Scan() {
		line := byLine.Text()
		line = strings.TrimSpace(line)

		if !strings.HasPrefix(line, "server ") {
			rows = append(rows, textBlock{value: line})
			continue
		}

		line = strings.TrimSuffix(line, ";")
		parts := strings.Split(line, " ")

		status := ""
		otherParams := []string{}

		if len(parts) > 2 {
			for _, part := range parts[2:] {
				part = strings.TrimSpace(part)
				if part == "down" {
					status = part
				} else {
					otherParams = append(otherParams, part)
				}
			}
		}

		svr := &ServerRow{
			Name:       parts[1],
			Parameters: otherParams,
			Status:     status,
		}

		servers = append(servers, svr)
		rows = append(rows, svr)
	}

	if err = byLine.Err(); err != nil {
		return nil, nil, err
	}

	return
}

func ReloadConfig() error {
	output, err := exec.Command("nginx", "-t").CombinedOutput()
	if err != nil {
		log.Println(string(output))
		return err
	}

	output, err = exec.Command("nginx", "-s", "reload").CombinedOutput()
	if err != nil {
		log.Println(string(output))
		return err
	}

	return nil
}
