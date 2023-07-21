package azureformrecognize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/ntbosscher/gobase/env"
	"io"
	"net/url"
	"time"
)

var pipe runtime.Pipeline
var subscriptionId string

var azureEndpoint string
var apiVersion string
var verbose = false

const (
	// PrebuiltRead extract text from documents.
	PrebuiltRead = "prebuilt-read"

	// PrebuiltLayout extract text and layout information from documents.
	PrebuiltLayout = "prebuilt-layout"

	// PrebuiltDocument extract text, layout, entities, and general key-value pairs from documents.
	PrebuiltDocument = "prebuilt-document"

	// PrebuiltBusinessCard extract key information from business cards.
	PrebuiltBusinessCard = "prebuilt-businessCard"

	// PrebuiltIdDocument extract key information from passports and ID cards.
	PrebuiltIdDocument = "prebuilt-idDocument"

	// PrebuiltInvoice extract key information from invoices.
	PrebuiltInvoice = "prebuilt-invoice"

	// PrebuiltReceipt extract key information from receipts.
	PrebuiltReceipt = "prebuilt-receipt"

	// PrebuiltTaxUsW2 extract key information from IRS US W2 tax forms (year 2018-2021).
	PrebuiltTaxUsW2 = "prebuilt-tax.us.w2"
)

func init() {
	// need to also set AZURE_CLIENT_ID, AZURE_CLIENT_SECRET, AZURE_TENANT_ID
	subscriptionId = env.Require("AZURE_SUBSCRIPTION_ID")
	verbose = env.OptionalBool("AZURE_FORM_RECOGNIZE_VERBOSE", false)
	module := env.Optional("AZURE_FORM_RECOGNIZE_TRACE_MODULE_NAME", "gobase_azureformrecognize")
	version := env.Optional("AZURE_FORM_RECOGNIZE_TRACE_MODULE_VERSION", "v1.0")
	apiVersion = env.Optional("AZURE_FORM_RECOGNIZE_API_VERSION", "2023-02-28-preview")
	azureEndpoint = env.Require("AZURE_FORM_RECOGNIZE_ENDPOINT") // e.g. hardwaresched.cognitiveservices.azure.com

	conOptions := &azcore.ClientOptions{}
	pipe = runtime.NewPipeline(module, version, runtime.PipelineOptions{}, conOptions)
}

func StartAnalyzeJob(ctx context.Context, body io.ReadSeekCloser, contentType string, model string) (string, error) {

	qr := url.Values{}
	qr.Set("locale", "en-US")
	qr.Set("api-version", apiVersion)
	qr.Set("stringIndexType", "utf16CodeUnit")

	u := fmt.Sprintf("https://%s/formrecognizer/documentModels/%s:analyze?%s", azureEndpoint, model, qr.Encode())
	rq, err := runtime.NewRequest(ctx, "POST", u)
	if err != nil {
		return "", err
	}

	rq.Raw().Header.Set("Ocp-Apim-Subscription-Key", subscriptionId)

	err = rq.SetBody(body, contentType)
	if err != nil {
		return "", err
	}

	rs, err := pipe.Do(rq)
	if err != nil {
		return "", err
	}

	defer rs.Body.Close()

	jobId := rs.Header.Get("apim-request-id")
	if len(jobId) > 0 && rs.StatusCode < 400 {
		return jobId, nil
	}

	content, _ := io.ReadAll(rs.Body)

	if verbose {
		fmt.Println("Azure Request Failed:")
		fmt.Println(u)
		fmt.Println()
		fmt.Println("Status: ", rs.StatusCode)
		fmt.Println(string(content))
		fmt.Println()
	}

	return "", errors.New(string(content))
}

func GetJob(ctx context.Context, jobId string, modelName string) (*JobResult, error) {
	qr := url.Values{}
	qr.Set("api-version", apiVersion)

	u := fmt.Sprintf("https://%s/formrecognizer/documentModels/%s/analyzeResults/%s?%s", azureEndpoint, modelName, jobId, qr.Encode())
	rq, err := runtime.NewRequest(ctx, "GET", u)
	if err != nil {
		return nil, err
	}

	rq.Raw().Header.Set("Ocp-Apim-Subscription-Key", subscriptionId)

	res, err := pipe.Do(rq)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()

	if res.StatusCode >= 400 {
		data, _ := io.ReadAll(res.Body)
		if verbose {
			fmt.Println("Azure request failed")
			fmt.Println(u)
			fmt.Println()
			fmt.Println("Status: ", res.StatusCode)
			fmt.Println(string(data))
			fmt.Println()
		}

		return nil, errors.New(string(data))
	}

	result := &JobResult{}
	err = json.NewDecoder(res.Body).Decode(result)

	return result, err
}

type JobResult struct {
	CreatedDateTime     time.Time
	LastUpdatedDateTime time.Time
	Status              string

	AnalyzeResult *AnalyzeResult
}

type AnalyzeResult struct {
	Tables []*AnalyzeTable
}

type AnalyzeTable struct {
	RowCount    int
	ColumnCount int
	Cells       []*AnalyzeCell
}

type AnalyzeCell struct {
	Kind        string
	RowIndex    int
	ColumnIndex int
	Content     string
	ColumnSpan  int
	RowSpan     int
}
