package testutil

// Sample file headers that http.DetectContentType recognizes. They are not
// complete images or documents: they exercise type detection only, so use them
// where the data is checked by its type and not decoded.
var (
	SamplePNG  = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01")
	SampleJPEG = []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00")
	SampleGIF  = []byte("GIF89a\x01\x00\x01\x00")
	SampleWebP = []byte("RIFF\x24\x00\x00\x00WEBPVP8 ")
	SamplePDF  = []byte("%PDF-1.4\n%sample\n")
	SampleBMP  = []byte("BM\x36\x00\x00\x00\x00\x00")
)
