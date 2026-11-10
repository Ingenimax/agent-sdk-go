package testutil

import (
	"context"
	"errors"
	"sync"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/storage"
)

// FakeImageEditSession is a scriptable interfaces.ImageEditSession that
// answers every SendMessage with a fixed response and error and records what
// it was sent. Build one with NewFakeImageEditSession.
type FakeImageEditSession struct {
	mu       sync.Mutex
	response *interfaces.ImageEditResponse
	err      error
	history  []interfaces.ImageEditTurn
	prompts  []string
	lastOpts *interfaces.ImageEditOptions
	closed   bool
}

// NewFakeImageEditSession returns a session that answers with resp and err.
// A nil resp is replaced by an empty response.
func NewFakeImageEditSession(resp *interfaces.ImageEditResponse, err error) *FakeImageEditSession {
	if resp == nil {
		resp = &interfaces.ImageEditResponse{}
	}
	return &FakeImageEditSession{response: resp, err: err}
}

// SendMessage implements interfaces.ImageEditSession.
func (s *FakeImageEditSession) SendMessage(_ context.Context, msg string, opts *interfaces.ImageEditOptions) (*interfaces.ImageEditResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prompts = append(s.prompts, msg)
	s.lastOpts = opts
	return s.response, s.err
}

// SendMessageWithImage implements interfaces.ImageEditSession. It is not
// scripted and always fails.
func (s *FakeImageEditSession) SendMessageWithImage(context.Context, string, *interfaces.ImageData, *interfaces.ImageEditOptions) (*interfaces.ImageEditResponse, error) {
	return nil, errors.New("FakeImageEditSession: SendMessageWithImage is not supported")
}

// GetHistory implements interfaces.ImageEditSession.
func (s *FakeImageEditSession) GetHistory() []interfaces.ImageEditTurn {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.history
}

// Close implements interfaces.ImageEditSession.
func (s *FakeImageEditSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

// SetErr changes the error returned by later SendMessage calls.
func (s *FakeImageEditSession) SetErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

// SetHistory sets the history returned by GetHistory.
func (s *FakeImageEditSession) SetHistory(history []interfaces.ImageEditTurn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = history
}

// Prompts returns every message sent, in order.
func (s *FakeImageEditSession) Prompts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.prompts...)
}

// LastOptions returns the options of the last SendMessage call.
func (s *FakeImageEditSession) LastOptions() *interfaces.ImageEditOptions {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastOpts
}

// Closed reports whether Close was called.
func (s *FakeImageEditSession) Closed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// FakeImageEditor is a scriptable interfaces.MultiTurnImageEditor.
//
// Each CreateImageEditSession call builds a session with NewSession, or an
// empty-response session when NewSession is nil, unless Err is set.
type FakeImageEditor struct {
	NewSession  func() *FakeImageEditSession
	Err         error
	Unsupported bool

	mu       sync.Mutex
	sessions []*FakeImageEditSession
	lastOpts *interfaces.ImageEditSessionOptions
}

// CreateImageEditSession implements interfaces.MultiTurnImageEditor.
func (e *FakeImageEditor) CreateImageEditSession(_ context.Context, opts *interfaces.ImageEditSessionOptions) (interfaces.ImageEditSession, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.lastOpts = opts
	if e.Err != nil {
		return nil, e.Err
	}
	s := NewFakeImageEditSession(nil, nil)
	if e.NewSession != nil {
		s = e.NewSession()
	}
	e.sessions = append(e.sessions, s)
	return s, nil
}

// SupportsMultiTurnImageEditing implements interfaces.MultiTurnImageEditor.
func (e *FakeImageEditor) SupportsMultiTurnImageEditing() bool { return !e.Unsupported }

// Sessions returns every session handed out, in order.
func (e *FakeImageEditor) Sessions() []*FakeImageEditSession {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]*FakeImageEditSession(nil), e.sessions...)
}

// LastOptions returns the options of the last CreateImageEditSession call.
func (e *FakeImageEditor) LastOptions() *interfaces.ImageEditSessionOptions {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastOpts
}

// FakeImageStorage is a storage.ImageStorage that returns
// "https://cdn.test/<mime type>" for every stored image, or Err when set.
type FakeImageStorage struct {
	Err error
	// StorageName is returned by Name; it defaults to "fake".
	StorageName string

	mu    sync.Mutex
	metas []storage.StorageMetadata
}

// Store implements storage.ImageStorage.
func (f *FakeImageStorage) Store(_ context.Context, img *interfaces.GeneratedImage, meta storage.StorageMetadata) (string, error) {
	if f.Err != nil {
		return "", f.Err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.metas = append(f.metas, meta)
	return "https://cdn.test/" + img.MimeType, nil
}

// Delete implements storage.ImageStorage.
func (f *FakeImageStorage) Delete(context.Context, string) error { return nil }

// Get implements storage.ImageStorage.
func (f *FakeImageStorage) Get(context.Context, string) ([]byte, error) { return nil, nil }

// Name implements storage.ImageStorage.
func (f *FakeImageStorage) Name() string {
	if f.StorageName == "" {
		return "fake"
	}
	return f.StorageName
}

// Metadata returns the metadata of every stored image, in order.
func (f *FakeImageStorage) Metadata() []storage.StorageMetadata {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]storage.StorageMetadata(nil), f.metas...)
}

var (
	_ interfaces.ImageEditSession     = (*FakeImageEditSession)(nil)
	_ interfaces.MultiTurnImageEditor = (*FakeImageEditor)(nil)
	_ storage.ImageStorage            = (*FakeImageStorage)(nil)
)
