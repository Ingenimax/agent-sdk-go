package testutil

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Ingenimax/agent-sdk-go/pkg/interfaces"
	"github.com/Ingenimax/agent-sdk-go/pkg/storage"
)

func TestFakeImageEditSession(t *testing.T) {
	ctx := context.Background()
	resp := &interfaces.ImageEditResponse{}
	s := NewFakeImageEditSession(resp, nil)

	opts := &interfaces.ImageEditOptions{}
	got, err := s.SendMessage(ctx, "first", nil)
	if err != nil || got != resp {
		t.Fatalf("SendMessage() = %v, %v, want the scripted response", got, err)
	}
	if _, err := s.SendMessage(ctx, "second", opts); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if p := s.Prompts(); !reflect.DeepEqual(p, []string{"first", "second"}) {
		t.Errorf("Prompts() = %v", p)
	}
	if s.LastOptions() != opts {
		t.Error("LastOptions() is not the last options passed")
	}

	sendErr := errors.New("send failed")
	s.SetErr(sendErr)
	if _, err := s.SendMessage(ctx, "third", nil); !errors.Is(err, sendErr) {
		t.Errorf("SendMessage() error = %v, want %v", err, sendErr)
	}

	if _, err := s.SendMessageWithImage(ctx, "x", nil, nil); err == nil {
		t.Error("SendMessageWithImage() error = nil, want an error")
	}

	history := []interfaces.ImageEditTurn{{}}
	s.SetHistory(history)
	if len(s.GetHistory()) != 1 {
		t.Errorf("GetHistory() = %v, want one turn", s.GetHistory())
	}

	if s.Closed() {
		t.Error("Closed() = true before Close")
	}
	if err := s.Close(); err != nil || !s.Closed() {
		t.Errorf("Close() = %v, Closed() = %v", err, s.Closed())
	}
}

func TestNewFakeImageEditSessionNilResponse(t *testing.T) {
	s := NewFakeImageEditSession(nil, nil)
	got, err := s.SendMessage(context.Background(), "x", nil)
	if err != nil || got == nil {
		t.Errorf("SendMessage() = %v, %v, want an empty response", got, err)
	}
}

func TestFakeImageEditor(t *testing.T) {
	ctx := context.Background()

	t.Run("default sessions", func(t *testing.T) {
		e := &FakeImageEditor{}
		opts := &interfaces.ImageEditSessionOptions{}
		s, err := e.CreateImageEditSession(ctx, opts)
		if err != nil || s == nil {
			t.Fatalf("CreateImageEditSession() = %v, %v", s, err)
		}
		if len(e.Sessions()) != 1 || e.Sessions()[0] != s {
			t.Errorf("Sessions() = %v, want the one session", e.Sessions())
		}
		if e.LastOptions() != opts {
			t.Error("LastOptions() is not the options passed")
		}
		if !e.SupportsMultiTurnImageEditing() {
			t.Error("SupportsMultiTurnImageEditing() = false, want true")
		}
	})

	t.Run("custom session", func(t *testing.T) {
		custom := NewFakeImageEditSession(nil, nil)
		e := &FakeImageEditor{NewSession: func() *FakeImageEditSession { return custom }}
		s, err := e.CreateImageEditSession(ctx, nil)
		if err != nil || s != custom {
			t.Errorf("CreateImageEditSession() = %v, %v, want the custom session", s, err)
		}
	})

	t.Run("error and unsupported", func(t *testing.T) {
		createErr := errors.New("create failed")
		e := &FakeImageEditor{Err: createErr, Unsupported: true}
		if _, err := e.CreateImageEditSession(ctx, nil); !errors.Is(err, createErr) {
			t.Errorf("CreateImageEditSession() error = %v, want %v", err, createErr)
		}
		if len(e.Sessions()) != 0 {
			t.Errorf("Sessions() = %v, want none", e.Sessions())
		}
		if e.SupportsMultiTurnImageEditing() {
			t.Error("SupportsMultiTurnImageEditing() = true, want false")
		}
	})
}

func TestFakeImageStorage(t *testing.T) {
	ctx := context.Background()
	img := &interfaces.GeneratedImage{MimeType: "image/png"}

	f := &FakeImageStorage{}
	meta := storage.StorageMetadata{OrgID: "org"}
	url, err := f.Store(ctx, img, meta)
	if err != nil || url != "https://cdn.test/image/png" {
		t.Errorf("Store() = %q, %v", url, err)
	}
	if m := f.Metadata(); len(m) != 1 || m[0].OrgID != "org" {
		t.Errorf("Metadata() = %+v", m)
	}
	if err := f.Delete(ctx, url); err != nil {
		t.Errorf("Delete() error = %v", err)
	}
	if data, err := f.Get(ctx, url); data != nil || err != nil {
		t.Errorf("Get() = %v, %v, want nil, nil", data, err)
	}
	if f.Name() != "fake" {
		t.Errorf("Name() = %q, want fake", f.Name())
	}

	storeErr := errors.New("store failed")
	named := &FakeImageStorage{Err: storeErr, StorageName: "custom"}
	if _, err := named.Store(ctx, img, meta); !errors.Is(err, storeErr) {
		t.Errorf("Store() error = %v, want %v", err, storeErr)
	}
	if named.Name() != "custom" {
		t.Errorf("Name() = %q, want custom", named.Name())
	}
}
