package openai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

func TestUserMessageSendsInlinePDFAsFile(t *testing.T) {
	pdf := interfaces.NewPartFromBytes([]byte{1, 2}, "application/pdf")
	pdf.InlineData.DisplayName = "report.pdf"
	raw, err := json.Marshal(userMessage("", []*interfaces.Part{pdf, interfaces.NewPartFromText("summarize")}))
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var got struct {
		Content []struct {
			Type string `json:"type"`
			File struct {
				FileData string `json:"file_data"`
				Filename string `json:"filename"`
			} `json:"file"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("content is not a list of parts: %s", raw)
	}
	if len(got.Content) != 2 || got.Content[0].Type != "file" {
		t.Fatalf("parts = %s, want a file part then text", raw)
	}
	if f := got.Content[0].File; f.FileData != "data:application/pdf;base64,AQI=" || f.Filename != "report.pdf" {
		t.Errorf("file = %+v, want the PDF as a data URL with its name", f)
	}
}

func TestCheckContentRefusesPDFByURL(t *testing.T) {
	inline := &interfaces.GenerateOptions{}
	interfaces.WithParts(interfaces.NewPartFromBytes([]byte{1}, "application/pdf"))(inline)
	if err := checkContent(context.Background(), inline); err != nil {
		t.Errorf("checkContent() with an inline PDF = %v", err)
	}

	byURL := &interfaces.GenerateOptions{}
	interfaces.WithParts(interfaces.NewPartFromURI("https://example.com/a.pdf", "application/pdf"))(byURL)
	if err := checkContent(context.Background(), byURL); !errors.Is(err, interfaces.ErrContentUnsupported) {
		t.Errorf("checkContent() with a PDF URL = %v, want ErrContentUnsupported", err)
	}
}
