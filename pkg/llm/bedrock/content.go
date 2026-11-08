package bedrock

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
)

// The Converse formats for each MIME type this client sends.
var (
	imageFormats = map[string]types.ImageFormat{
		"image/png":  types.ImageFormatPng,
		"image/jpeg": types.ImageFormatJpeg,
		"image/gif":  types.ImageFormatGif,
		"image/webp": types.ImageFormatWebp,
	}
	documentFormats = map[string]types.DocumentFormat{
		"application/pdf":    types.DocumentFormatPdf,
		"text/csv":           types.DocumentFormatCsv,
		"application/msword": types.DocumentFormatDoc,
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": types.DocumentFormatDocx,
		"application/vnd.ms-excel": types.DocumentFormatXls,
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": types.DocumentFormatXlsx,
		"text/html":     types.DocumentFormatHtml,
		"text/plain":    types.DocumentFormatTxt,
		"text/markdown": types.DocumentFormatMd,
	}
	videoFormats = map[string]types.VideoFormat{
		"video/x-matroska": types.VideoFormatMkv,
		"video/quicktime":  types.VideoFormatMov,
		"video/mp4":        types.VideoFormatMp4,
		"video/webm":       types.VideoFormatWebm,
		"video/x-flv":      types.VideoFormatFlv,
		"video/mpeg":       types.VideoFormatMpeg,
		"video/x-ms-wmv":   types.VideoFormatWmv,
		"video/3gpp":       types.VideoFormatThreeGp,
	}
	audioFormats = map[string]types.AudioFormat{
		"audio/mpeg":       types.AudioFormatMp3,
		"audio/wav":        types.AudioFormatWav,
		"audio/flac":       types.AudioFormatFlac,
		"audio/ogg":        types.AudioFormatOgg,
		"audio/aac":        types.AudioFormatAac,
		"audio/opus":       types.AudioFormatOpus,
		"audio/mp4":        types.AudioFormatMp4,
		"audio/webm":       types.AudioFormatWebm,
		"audio/x-matroska": types.AudioFormatMka,
		"audio/x-wav":      types.AudioFormatWav,
		"audio/mp3":        types.AudioFormatMp3,
		"audio/x-flac":     types.AudioFormatFlac,
		"audio/x-m4a":      types.AudioFormatM4a,
	}
)

// errDocumentName is returned for a document without a usable name.
var errDocumentName = errors.New("bedrock: a document needs a DisplayName with at least one letter, digit, hyphen, parenthesis, or square bracket")

// partBlocks converts parts to Converse blocks, in order. checkContent has
// already refused every part that is not text or a supported image,
// document, video, or audio part.
func partBlocks(parts []*interfaces.Part) []types.ContentBlock {
	blocks := make([]types.ContentBlock, 0, len(parts))
	for _, p := range parts {
		mime := p.MIMEType()
		switch {
		case p.IsText():
			blocks = append(blocks, &types.ContentBlockMemberText{Value: p.Text})
		case imageFormats[mime] != "":
			var source types.ImageSource
			if p.InlineData != nil {
				source = &types.ImageSourceMemberBytes{Value: p.InlineData.Data}
			} else {
				source = &types.ImageSourceMemberS3Location{Value: s3Location(p)}
			}
			blocks = append(blocks, &types.ContentBlockMemberImage{Value: types.ImageBlock{Format: imageFormats[mime], Source: source}})
		case documentFormats[mime] != "":
			var source types.DocumentSource
			if p.InlineData != nil {
				source = &types.DocumentSourceMemberBytes{Value: p.InlineData.Data}
			} else {
				source = &types.DocumentSourceMemberS3Location{Value: s3Location(p)}
			}
			blocks = append(blocks, &types.ContentBlockMemberDocument{Value: types.DocumentBlock{
				Format: documentFormats[mime],
				Name:   aws.String(documentName(p)),
				Source: source,
			}})
		case videoFormats[mime] != "":
			var source types.VideoSource
			if p.InlineData != nil {
				source = &types.VideoSourceMemberBytes{Value: p.InlineData.Data}
			} else {
				source = &types.VideoSourceMemberS3Location{Value: s3Location(p)}
			}
			blocks = append(blocks, &types.ContentBlockMemberVideo{Value: types.VideoBlock{Format: videoFormats[mime], Source: source}})
		case audioFormats[mime] != "":
			var source types.AudioSource
			if p.InlineData != nil {
				source = &types.AudioSourceMemberBytes{Value: p.InlineData.Data}
			} else {
				source = &types.AudioSourceMemberS3Location{Value: s3Location(p)}
			}
			blocks = append(blocks, &types.ContentBlockMemberAudio{Value: types.AudioBlock{Format: audioFormats[mime], Source: source}})
		}
	}
	return blocks
}

// s3Location returns the S3 location of a file data part.
func s3Location(p *interfaces.Part) types.S3Location {
	return types.S3Location{Uri: aws.String(p.FileData.FileURI)}
}

// maxDocumentName is the longest document name Converse accepts.
const maxDocumentName = 200

// documentName returns the part's display name in the form Converse
// accepts: ASCII letters and digits, hyphens, parentheses, square brackets,
// and single spaces, at most 200 characters. Every other character becomes a
// space, so "report.pdf" becomes "report pdf". It returns "" when nothing
// usable is left.
func documentName(p *interfaces.Part) string {
	cleaned := strings.Map(func(r rune) rune {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) || strings.ContainsRune("-()[]", r) {
			return r
		}
		return ' '
	}, displayName(p))
	name := strings.Join(strings.Fields(cleaned), " ")
	if len(name) > maxDocumentName {
		name = strings.TrimSpace(name[:maxDocumentName])
	}
	return name
}

// supportedPart reports whether Converse takes the part: a known image,
// document, video, or audio MIME type, sent inline or from an s3:// URI.
func supportedPart(p *interfaces.Part) bool {
	if p.FileData != nil && !strings.HasPrefix(p.FileData.FileURI, "s3://") {
		return false
	}
	mime := p.MIMEType()
	return imageFormats[mime] != "" || documentFormats[mime] != "" || videoFormats[mime] != "" || audioFormats[mime] != ""
}

// checkContent refuses every part of the request that Converse cannot take.
// It also refuses what Converse rejects about documents: a document without
// a usable name, two documents with the same name, and a user message that
// has a document but no text.
func checkContent(ctx context.Context, params *interfaces.GenerateOptions) error {
	if err := interfaces.CheckMedia(ctx, params, providerName, supportedPart); err != nil {
		return err
	}
	turns, err := userTurns(ctx, params)
	if err != nil {
		return err
	}
	names := map[string]bool{}
	for _, parts := range turns {
		hasDocument := false
		for _, p := range parts {
			if p.IsText() || documentFormats[p.MIMEType()] == "" {
				continue
			}
			hasDocument = true
			name := documentName(p)
			switch {
			case name == "":
				return fmt.Errorf("%w (got %q)", errDocumentName, displayName(p))
			case names[name]:
				return fmt.Errorf("bedrock: two documents are named %q; Converse needs unique document names", name)
			}
			names[name] = true
		}
		if hasDocument && strings.TrimSpace(interfaces.PartsText(parts)) == "" {
			return errors.New("bedrock: a message with a document needs a text part too")
		}
	}
	return nil
}

// userTurns returns the parts of each user turn sent as parts: the parts set
// with WithParts when there is no memory, or the parts of each user message
// in memory.
func userTurns(ctx context.Context, params *interfaces.GenerateOptions) ([][]*interfaces.Part, error) {
	if params.Memory == nil {
		return [][]*interfaces.Part{params.Parts}, nil
	}
	messages, err := params.Memory.GetMessages(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read memory: %w", err)
	}
	var turns [][]*interfaces.Part
	for _, m := range messages {
		if m.Role == interfaces.MessageRoleUser && len(m.Parts) > 0 {
			turns = append(turns, m.Parts)
		}
	}
	return turns, nil
}

// displayName returns the display name of inline or file data.
func displayName(p *interfaces.Part) string {
	if p.InlineData != nil {
		return p.InlineData.DisplayName
	}
	return p.FileData.DisplayName
}
