package azureopenai

import (
	"context"
	"errors"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

func TestCheckContentRefusesPDFs(t *testing.T) {
	for _, part := range []*interfaces.Part{
		interfaces.NewPartFromBytes([]byte{1}, "application/pdf"),
		interfaces.NewPartFromURI("https://example.com/a.pdf", "application/pdf"),
	} {
		params := &interfaces.GenerateOptions{}
		interfaces.WithParts(part)(params)
		if err := checkContent(context.Background(), params); !errors.Is(err, interfaces.ErrContentUnsupported) {
			t.Errorf("checkContent() with a PDF = %v, want ErrContentUnsupported", err)
		}
	}
}
