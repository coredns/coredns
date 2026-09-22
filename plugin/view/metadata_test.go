package view

import (
	"context"
	"testing"

	"github.com/coredns/coredns/plugin/metadata"
	"github.com/coredns/coredns/request"
)

func TestMetadata_PublishesViewName(t *testing.T) {
	v := &View{viewName: "myview"}
	ctx := metadata.ContextWithMetadata(context.Background())
	st := request.Request{}
	ctx = v.Metadata(ctx, st)

	if f := metadata.ValueFunc(ctx, "view/name"); f == nil {
		t.Fatalf("metadata value func is nil")
	} else if got := f(); got != "myview" {
		t.Fatalf("metadata value: got %q, want %q", got, "myview")
	}
}
