package api

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestClientWithContextCancelsRequest(t *testing.T) {
	client := NewClientWithHTTPClient(&http.Client{}, "userid=test")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	requestClient := client.WithContext(ctx)
	if requestClient == client {
		t.Fatal("WithContext 应返回请求级浅拷贝")
	}
	if client.ctx != nil {
		t.Fatal("WithContext 不应修改共享客户端")
	}
	if _, err := requestClient.post("/test", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("期望 context canceled，实际 %v", err)
	}
}
